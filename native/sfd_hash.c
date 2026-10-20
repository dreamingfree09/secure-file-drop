#include "sfd_hash.h"

#include <errno.h>
#include <stdio.h>
#include <string.h>

#include <openssl/evp.h>

void sfd_hex_encode_lower(const uint8_t *bytes, size_t len, char *out_hex) {
    static const char *hex = "0123456789abcdef";
    for (size_t i = 0; i < len; i++) {
        out_hex[i * 2]     = hex[(bytes[i] >> 4) & 0x0F];
        out_hex[i * 2 + 1] = hex[bytes[i] & 0x0F];
    }
    out_hex[len * 2] = '\0';
}

int sfd_sha256_file(const char *path, uint8_t out_hash[SFD_SHA256_LEN], uint64_t *out_bytes) {
    if (path == NULL || out_hash == NULL || out_bytes == NULL) {
        return 1;
    }

    *out_bytes = 0;

    FILE *f = fopen(path, "rb");
    if (!f) {
        return 2;
    }

    EVP_MD_CTX *ctx = EVP_MD_CTX_new();
    if (!ctx) {
        fclose(f);
        return 3;
    }

    if (EVP_DigestInit_ex(ctx, EVP_sha256(), NULL) != 1) {
        EVP_MD_CTX_free(ctx);
        fclose(f);
        return 3;
    }

    unsigned char buf[64 * 1024];
    while (1) {
        size_t n = fread(buf, 1, sizeof(buf), f);
        if (n > 0) {
            if (EVP_DigestUpdate(ctx, buf, n) != 1) {
                EVP_MD_CTX_free(ctx);
                fclose(f);
                return 3;
            }
            *out_bytes += (uint64_t)n;
        }

        if (n < sizeof(buf)) {
            if (feof(f)) {
                break;
            }
            if (ferror(f)) {
                EVP_MD_CTX_free(ctx);
                fclose(f);
                return 2;
            }
        }
    }

    unsigned int md_len = 0;
    unsigned char md[EVP_MAX_MD_SIZE];

    if (EVP_DigestFinal_ex(ctx, md, &md_len) != 1) {
        EVP_MD_CTX_free(ctx);
        fclose(f);
        return 3;
    }

    EVP_MD_CTX_free(ctx);
    fclose(f);

    if (md_len != SFD_SHA256_LEN) {
        return 3;
    }

    memcpy(out_hash, md, SFD_SHA256_LEN);
    return 0;
}
