#include "sfd_hash.h"

#include <stdio.h>
#include <stdint.h>
#include <string.h>

#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

static void usage(const char *prog) {
    fprintf(stderr, "Usage: %s <file-path|--stdin>\n", prog);
}

int main(int argc, char **argv) {
    if (argc != 2) {
        usage(argv[0]);
        return 1; /* usage error */
    }

    uint8_t hash[SFD_SHA256_LEN];
    uint64_t bytes = 0;

    int rc;
    if (strcmp(argv[1], "--stdin") == 0) {
#ifdef _WIN32
        if (_setmode(_fileno(stdin), _O_BINARY) == -1) {
            fprintf(stderr, "Stream I/O error\n");
            return 2;
        }
#endif
        rc = sfd_sha256_stream(stdin, hash, &bytes);
    } else {
        rc = sfd_sha256_file(argv[1], hash, &bytes);
    }
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
    if (printf("{\"algorithm\":\"sha256\",\"hash\":\"%s\",\"bytes\":%llu}\n",
           hex, (unsigned long long)bytes) < 0 || fflush(stdout) == EOF) {
        fprintf(stderr, "Output I/O error\n");
        return 2;
    }

    return 0;
}
