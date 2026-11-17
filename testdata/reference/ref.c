/* Reference: encodes or decodes with the original C code (64 kbit/s, 16 kHz).
   ref enc in.pcm out.g722 | ref dec in.g722 out.pcm; PCM: int16 little endian. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include "g722.h"

static unsigned char *slurp(const char *p, long *n) {
    FILE *f = fopen(p, "rb"); if (!f) { perror(p); exit(1); }
    fseek(f, 0, SEEK_END); *n = ftell(f); fseek(f, 0, SEEK_SET);
    unsigned char *b = malloc(*n); fread(b, 1, *n, f); fclose(f); return b;
}

int main(int argc, char **argv) {
    if (argc != 4) return 2;
    long n; unsigned char *in = slurp(argv[2], &n);
    FILE *out = fopen(argv[3], "wb");
    if (strcmp(argv[1], "enc") == 0) {
        g722_encode_state_t s; g722_encode_init(&s, 64000, 0);
        uint8_t *o = malloc(n / 4 + 1);
        int k = g722_encode(&s, o, (int16_t *)in, n / 2);
        fwrite(o, 1, k, out);
    } else {
        g722_decode_state_t s; g722_decode_init(&s, 64000, 0);
        int16_t *o = malloc(n * 4);
        int k = g722_decode(&s, o, in, n);
        fwrite(o, 2, k, out);
    }
    fclose(out);
    return 0;
}
