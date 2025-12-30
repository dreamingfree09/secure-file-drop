# Admin Password Migration Guide

## ⚠️ BREAKING CHANGE: Admin Password Now Requires bcrypt Hash

As of this security update, the `SFD_ADMIN_PASS` environment variable **must** be a bcrypt hash, not a plaintext password.

---

## Why This Change?

**Security Issue**: The previous implementation used SHA256 for admin password comparison, which is:
- Fast and vulnerable to brute-force attacks
- Inconsistent with user passwords (which use bcrypt)
- Not recommended for password storage

**Fix**: Admin authentication now uses bcrypt (same as user passwords) with cost factor 12.

---

## How to Generate a bcrypt Hash

### Option 1: Using htpasswd (Recommended)

```bash
# Generate bcrypt hash for password "yourpassword"
htpasswd -bnBC 12 "" yourpassword | tr -d ':'

# Example output:
# $2y$12$abc123...xyz789
```

### Option 2: Using Python

```python
import bcrypt

password = "yourpassword"
hash = bcrypt.hashpw(password.encode(), bcrypt.gensalt(rounds=12))
print(hash.decode())
```

### Option 3: Using Go

```go
package main

import (
    "fmt"
    "golang.org/x/crypto/bcrypt"
)

func main() {
    password := "yourpassword"
    hash, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
    fmt.Println(string(hash))
}
```

### Option 4: Using Node.js

```javascript
const bcrypt = require('bcryptjs');

const password = "yourpassword";
const hash = bcrypt.hashSync(password, 12);
console.log(hash);
```

---

## Setting the Environment Variable

### Docker Compose

```yaml
services:
  backend:
    environment:
      SFD_ADMIN_PASS: "$2y$12$abc123...xyz789"  # Your bcrypt hash
```

### Kubernetes Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sfd-secrets
stringData:
  SFD_ADMIN_PASS: "$2y$12$abc123...xyz789"  # Your bcrypt hash
```

### Direct Environment Variable

```bash
export SFD_ADMIN_PASS='$2y$12$abc123...xyz789'
```

**Important**: Use single quotes to prevent shell expansion of `$` characters!

---

## Validation on Startup

The application now validates that `SFD_ADMIN_PASS` is a valid bcrypt hash. It will **refuse to start** if:

1. The value doesn't start with `$2a$`, `$2b$`, or `$2y$` (bcrypt prefixes)
2. The value is missing or empty

**Error message if invalid**:
```
SECURITY ERROR: SFD_ADMIN_PASS must be a bcrypt hash
Example: htpasswd -bnBC 12 '' yourpassword | tr -d ':'
```

---

## Migration Steps

### For Existing Deployments

1. **Generate bcrypt hash** for your current admin password:
   ```bash
   htpasswd -bnBC 12 "" your-current-password | tr -d ':'
   ```

2. **Update your deployment**:
   - Docker Compose: Update `docker-compose.yml`
   - Kubernetes: Update secret with `kubectl edit secret sfd-secrets`
   - Environment: Update `.env` or export statement

3. **Restart the application**:
   ```bash
   # Docker Compose
   docker-compose restart backend
   
   # Kubernetes
   kubectl rollout restart deployment/sfd-backend -n secure-file-drop
   ```

4. **Verify login** works with the same password as before

---

## Security Benefits

✅ **Resistant to brute-force**: bcrypt is computationally expensive (cost 12 = ~300ms per attempt)  
✅ **Consistent security**: Same algorithm for all passwords (admin + users)  
✅ **Industry standard**: bcrypt is the recommended algorithm for password hashing  
✅ **Future-proof**: Cost factor can be increased as hardware improves  

---

## Troubleshooting

### Error: "SECURITY ERROR: SFD_ADMIN_PASS must be a bcrypt hash"

**Cause**: The environment variable contains a plaintext password or invalid hash.

**Solution**: Generate a bcrypt hash using one of the methods above.

---

### Login fails after migration

**Possible causes**:
1. Hash was not generated correctly
2. Shell interpreted `$` characters (use single quotes!)
3. Hash was truncated or modified

**Debug steps**:
```bash
# Test your hash generation
htpasswd -bnBC 12 "" testpassword | tr -d ':'

# Verify the hash is set correctly (first 10 chars)
echo "$SFD_ADMIN_PASS" | head -c 10
# Should show: $2y$12$ or similar
```

---

### Can I use the old plaintext method?

**No**. The plaintext/SHA256 method has been removed for security reasons. You must migrate to bcrypt hashes.

---

## Example: Complete Migration

```bash
# 1. Generate hash for password "MySecureP@ss123"
HASH=$(htpasswd -bnBC 12 "" 'MySecureP@ss123' | tr -d ':')

# 2. Update Docker Compose
cat >> docker-compose.yml << EOF
services:
  backend:
    environment:
      SFD_ADMIN_PASS: "$HASH"
EOF

# 3. Restart
docker-compose up -d backend

# 4. Test login with username "admin" and password "MySecureP@ss123"
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"MySecureP@ss123"}'
```

---

## Related Documentation

- [SECURITY_FIXES.md](SECURITY_FIXES.md) - All security updates
- [README.md](../README.md) - Main project documentation
- [SPEC.md](SPEC.md) - API specification

---

**Last Updated**: December 30, 2025  
**Migration Required By**: Before next deployment  
**Impact**: All deployments using `SFD_ADMIN_PASS`
