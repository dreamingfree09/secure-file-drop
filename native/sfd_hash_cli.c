#include "sfd_hash.h"

#include <stdio.h>
#include <stdint.h>

static void usage(const char *prog) {
    fprintf(stderr, "Usage: %s <file-path>\n", prog);
}

int main(int argc, char **argv) {
    if (argc != 2) {
        usage(argv[0]);
        return 1; /* usage error */
    }

    const char *path = argv[1];

    uint8_t hash[SFD_SHA256_LEN];
    uint64_t bytes = 0;

    int rc = sfd_sha256_file(path, hash, &bytes);
    if (rc != 0) {
        if (rc == 2) {
            fprintf(stderr, "File I/O error\n");
            return 2;
        }
        fprintf(stderr, "Hashing error\n");
        return 3;
    }

    char hex[SFD_SHA256_LEN * 2 + 1];
    sfd_hex_encode_lower(hash, SFD_SHA256_LEN, hex);

    /* Deterministic JSON output (stdout) */
    printf("{\"algorithm\":\"sha256\",\"hash\":\"%s\",\"bytes\":%llu}\n",
           hex, (unsigned long long)bytes);

    return 0;
}
