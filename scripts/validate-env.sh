#!/bin/bash
# .env validation script
# Checks that all required environment variables are set and secrets are properly configured

set -e

ENV_FILE="${1:-.env}"

if [ ! -f "$ENV_FILE" ]; then
    echo "❌ Error: $ENV_FILE not found"
    echo "💡 Tip: Copy .env.example to .env and fill in the values"
    exit 1
fi

echo "🔍 Validating $ENV_FILE..."
echo ""

# Source the env file
set -a
source "$ENV_FILE"
set +a

ERRORS=0
WARNINGS=0

# Function to check if a variable is set and not a placeholder
check_required() {
    local var_name="$1"
    local var_value="${!var_name}"
    local allow_empty="${2:-false}"
    
    if [ -z "$var_value" ]; then
        if [ "$allow_empty" = "false" ]; then
            echo "❌ $var_name is not set or empty"
            ((ERRORS++))
        fi
    elif [[ "$var_value" == *"CHANGE"* ]]; then
        echo "❌ $var_name still contains placeholder: $var_value"
        ((ERRORS++))
    else
        echo "✅ $var_name is set"
    fi
}

# Function to check secret strength
check_secret_strength() {
    local var_name="$1"
    local var_value="${!var_name}"
    local min_length="${2:-32}"
    
    if [ -z "$var_value" ]; then
        return
    fi
    
    if [ ${#var_value} -lt $min_length ]; then
        echo "⚠️  $var_name is shorter than recommended ($min_length chars)"
        ((WARNINGS++))
    fi
}

echo "📋 Required Variables:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Database
check_required "POSTGRES_DB"
check_required "POSTGRES_USER"
check_required "POSTGRES_PASSWORD"
check_required "DATABASE_URL"

# MinIO
check_required "MINIO_ROOT_USER"
check_required "MINIO_ROOT_PASSWORD"
check_required "SFD_BUCKET"

# Backend
check_required "SFD_ADMIN_USER"
check_required "SFD_ADMIN_PASS"
check_required "SFD_SESSION_SECRET"
check_required "SFD_DOWNLOAD_SECRET"

# Optional
check_required "SFD_PUBLIC_BASE_URL" "true"
check_required "SFD_MAX_UPLOAD_BYTES" "true"

echo ""
echo "🔐 Secret Strength Checks:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

check_secret_strength "SFD_SESSION_SECRET" 64
check_secret_strength "SFD_DOWNLOAD_SECRET" 64
check_secret_strength "SFD_ADMIN_PASS" 24
check_secret_strength "POSTGRES_PASSWORD" 24
check_secret_strength "MINIO_ROOT_PASSWORD" 24

echo ""
echo "🔬 Advanced Checks:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Check DATABASE_URL format
if [[ ! "$DATABASE_URL" =~ ^postgres(ql)?:// ]]; then
    echo "⚠️  DATABASE_URL does not start with postgres:// or postgresql://"
    ((WARNINGS++))
else
    echo "✅ DATABASE_URL format looks correct"
fi

# Check DATABASE_URL contains the right credentials
if [[ "$DATABASE_URL" == *"$POSTGRES_USER"* ]] && [[ "$DATABASE_URL" == *"$POSTGRES_PASSWORD"* ]]; then
    echo "✅ DATABASE_URL contains matching credentials"
else
    echo "⚠️  DATABASE_URL credentials may not match POSTGRES_USER/POSTGRES_PASSWORD"
    ((WARNINGS++))
fi

# Check SFD_MAX_UPLOAD_BYTES is a number if set
if [ -n "$SFD_MAX_UPLOAD_BYTES" ]; then
    if [[ "$SFD_MAX_UPLOAD_BYTES" =~ ^[0-9]+$ ]]; then
        echo "✅ SFD_MAX_UPLOAD_BYTES is a valid number: $SFD_MAX_UPLOAD_BYTES"
    else
        echo "❌ SFD_MAX_UPLOAD_BYTES must be a number: $SFD_MAX_UPLOAD_BYTES"
        ((ERRORS++))
    fi
fi

# Check SFD_PUBLIC_BASE_URL is a valid URL if set
if [ -n "$SFD_PUBLIC_BASE_URL" ]; then
    if [[ "$SFD_PUBLIC_BASE_URL" =~ ^https?:// ]]; then
        echo "✅ SFD_PUBLIC_BASE_URL is a valid URL"
    else
        echo "⚠️  SFD_PUBLIC_BASE_URL should start with http:// or https://"
        ((WARNINGS++))
    fi
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ $ERRORS -gt 0 ]; then
    echo "❌ Validation failed with $ERRORS error(s) and $WARNINGS warning(s)"
    echo ""
    echo "💡 To fix errors:"
    echo "   1. Copy .env.example to .env if you haven't already"
    echo "   2. Generate secrets with:"
    echo "      openssl rand -hex 32    # for SESSION_SECRET and DOWNLOAD_SECRET"
    echo "      openssl rand -base64 24 # for passwords"
    echo "   3. Replace all CHANGE_ME placeholders"
    exit 1
elif [ $WARNINGS -gt 0 ]; then
    echo "⚠️  Validation passed with $WARNINGS warning(s)"
    echo "   Your configuration will work, but consider addressing the warnings for production use"
    exit 0
else
    echo "✅ All checks passed! Your .env file is properly configured."
    exit 0
fi
