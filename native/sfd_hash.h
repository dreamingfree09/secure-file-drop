#ifndef SFD_HASH_H
#define SFD_HASH_H

#include <stddef.h>
#include <stdint.h>

#define SFD_SHA256_LEN 32

/* Computes SHA-256 for the file at `path`.
   On success: returns 0, writes 32 bytes to out_hash, and sets *out_bytes to total bytes read.
   On failure: returns non-zero. */
int sfd_sha256_file(const char *path, uint8_t out_hash[SFD_SHA256_LEN], uint64_t *out_bytes);

/* Converts binary hash to lowercase hex string. `out_hex` must be at least (SFD_SHA256_LEN*2 + 1). */
void sfd_hex_encode_lower(const uint8_t *bytes, size_t len, char *out_hex);

#endif /* SFD_HASH_H */
