package g722

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ITU-T G.722 Appendix II test sequences check the sub-band ADPCM
// encoder and decoder with the filter bank bypassed. They are copyrighted
// by the ITU and therefore not part of this repository; the tests run when
// G722_ITU_DIR points to a directory with the extracted files (see
// README.md). Only mode 1 (64 kbit/s) applies to this package.

func ituDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("G722_ITU_DIR")
	if dir == "" {
		t.Skip("G722_ITU_DIR not set; see README.md")
	}
	return dir
}

// readITU reads a test sequence file: two comment lines, then lines of 16
// hexadecimal 16-bit words followed by a checksum byte, then a closing
// comment line.
func readITU(t *testing.T, dir, name string) []uint16 {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, name))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	var words []uint16
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		// Comment lines, and the DOS end-of-file marker 0x1A after the
		// closing comment.
		if strings.HasPrefix(text, "/*") || text == "" || text == "\x1a" {
			continue
		}
		require.Len(t, text, 66, "%s:%d: line length", name, line)
		// Checksum: two's complement of the low byte of the sum of all
		// preceding characters.
		var sum byte
		for i := range 64 {
			sum += text[i]
		}
		want, err := strconv.ParseUint(text[64:], 16, 8)
		require.NoError(t, err)
		require.Equal(t, byte(want), -sum, "%s:%d: checksum", name, line)
		for i := 0; i < 64; i += 4 {
			w, err := strconv.ParseUint(text[i:i+4], 16, 16)
			require.NoError(t, err)
			words = append(words, uint16(w))
		}
	}
	require.NoError(t, sc.Err())
	return words
}

// Configuration 1: the encoder input X# carries the sample in the upper 15
// bits and the reset signal in the LSB; both sub-band encoders get the same
// sample. The output I# carries IH, IL in the upper byte and the reset
// signal in the LSB.
func TestITUEncoder(t *testing.T) {
	dir := ituDir(t)
	for _, tc := range []struct{ in, want string }{
		{"T1C1.XMT", "T2R1.COD"},
		{"T1C2.XMT", "T2R2.COD"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			in := readITU(t, dir, tc.in)
			want := readITU(t, dir, tc.want)
			require.Len(t, want, len(in))
			bands := newBands()
			got := make([]uint16, len(in))
			for i, x := range in {
				if x&1 == 1 {
					bands = newBands()
					got[i] = 1
					continue
				}
				xl := int(int16(x)) >> 1
				il := bands[0].encodeLow(xl)
				ih := bands[1].encodeHigh(xl)
				got[i] = uint16(ih<<6|il) << 8
			}
			assert.Equal(t, want, got)
		})
	}
}

// Configuration 2, mode 1: the decoder input I# carries IH, ILR in the
// upper byte; the outputs RL# and RH# carry the reconstructed sub-band
// signals shifted left by one bit, with the LSB marking invalid data.
func TestITUDecoder(t *testing.T) {
	dir := ituDir(t)
	for _, tc := range []struct{ in, low, high string }{
		{"T2R1.COD", "T3L1.RC1", "T3H1.RC0"},
		{"T2R2.COD", "T3L2.RC1", "T3H2.RC0"},
		{"T1D3.COD", "T3L3.RC1", "T3H3.RC0"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			in := readITU(t, dir, tc.in)
			wantLow := readITU(t, dir, tc.low)
			wantHigh := readITU(t, dir, tc.high)
			require.Len(t, wantLow, len(in))
			require.Len(t, wantHigh, len(in))
			bands := newBands()
			gotLow := make([]uint16, len(in))
			gotHigh := make([]uint16, len(in))
			for i, w := range in {
				if w&1 == 1 {
					bands = newBands()
					gotLow[i], gotHigh[i] = 1, 1
					continue
				}
				rl := bands[0].decodeLow(int(w>>8) & 63)
				rh := bands[1].decodeHigh(int(w >> 14))
				gotLow[i] = uint16(int16(rl << 1))  //nolint:gosec // RL is limited to 15 bits
				gotHigh[i] = uint16(int16(rh << 1)) //nolint:gosec // RH is limited to 15 bits
			}
			assert.Equal(t, wantLow, gotLow, "lower sub-band")
			assert.Equal(t, wantHigh, gotHigh, "upper sub-band")
		})
	}
}
