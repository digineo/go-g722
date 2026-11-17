# go-g722

G.722 encoder and decoder in pure Go, without cgo and without dependencies.

It supports what VoIP usually needs: 64 kbit/s, 16 kHz, 16-bit linear PCM, mono. Every two samples become one byte, so a 20 ms RTP packet carries 160 bytes for 320 samples.

```go
enc := g722.NewEncoder()
dec := g722.NewDecoder()

payload := make([]byte, 160)
enc.Encode(payload, pcm) // pcm: 320 samples

pcm = make([]int16, 320)
dec.Decode(pcm, payload)
```

Encoders and decoders are stateful; each audio stream needs its own.

Note a historical quirk of RTP (RFC 3551): G.722 is announced as `G722/8000` and the RTP timestamp runs at 8 kHz, although the audio is sampled at 16 kHz.

## Origin

The code is a translation of [dgoncharov/g722tools](https://github.com/dgoncharov/g722tools) (commit `b5e611c`, BSD-3-Clause), which is based on Steve Underwood's G.722 implementation (public domain) and the 1993 codec of Carnegie Mellon University. See [LICENSE](LICENSE) for the license and notices.

Differences from the original:

- The decoder clamps its output to the int16 range. The original truncates and wraps around at full scale, which is audible as loud clicks.
- Only 64 kbit/s at 16 kHz; the 56 and 48 kbit/s modes, 8 kHz mode and packed data are left out.

## Tests

Encoder and decoder are bit-exact with the original C code (`testdata/encoded.g722`, `testdata/decoded.pcm`). To regenerate the test data:

```sh
go run testdata/reference/gen.go    # testdata/input.pcm
# build ref.c with the sources from g722tools/src:
cc -D__inline__=inline -I g722tools/src -o ref testdata/reference/ref.c \
  g722tools/src/g722_encode.c g722tools/src/g722_decode.c
./ref enc testdata/input.pcm testdata/encoded.g722
./ref dec testdata/encoded.g722 testdata/decoded.pcm
```

### ITU test sequences

The code passes the digital test sequences of [ITU-T G.722 Appendix II](https://www.itu.int/rec/T-REC-G.722-198703-S!AppII/en) that apply to 64 kbit/s: the encoder test (configuration 1) and the decoder test in mode 1 (configuration 2). These sequences bypass the filter bank; it is covered by the comparison with the C code above.

The sequences are copyrighted by the ITU and not part of this repository, so `itu_test.go` is skipped unless `G722_ITU_DIR` points to the extracted files:

```sh
curl -sSL -o g722-appII.zip \
  'https://www.itu.int/rec/dologin_pub.asp?lang=e&id=T-REC-G.722-198703-S!AppII!ZPF-E&type=items'
unzip g722-appII.zip
unzip software/G722ap2/G722E/Software.zip -d itu
G722_ITU_DIR=itu/software/G722ap2/G722E go test -run ITU -v ./...
```
