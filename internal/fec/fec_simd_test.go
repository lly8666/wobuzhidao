package fec

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFECMulEntireGF256AndRuntimeFallback(t *testing.T) {
	fastMulOnce.Do(initFastMul)
	t.Logf("fec SIMD capability=%v; wbd_fec_scalar/noasm builds must report false", fecSIMDEnabled)
	for c := 0; c < 256; c++ {
		for v := 0; v < 256; v++ {
			in := []byte{byte(v)}
			got := []byte{0x5a}
			xorMul(got, in, byte(c))
			want := byte(0x5a) ^ gfMul(byte(c), byte(v))
			if got[0] != want {
				t.Fatalf("c=%d v=%d got=%x want=%x", c, v, got[0], want)
			}
		}
	}
}

func TestFECMulShortTailsUnalignedAndSentinels(t *testing.T) {
	fastMulOnce.Do(initFastMul)
	sizes := []int{1, 15, 16, 17, 31, 32, 33, 63, 64, 65, 96, 127, 128, 129, 256, 512, 1000, 1400, 4096}
	for _, n := range sizes {
		for _, offset := range []int{0, 1, 3, 7} {
			for _, coef := range []byte{0, 1, 2, 127, 255} {
				t.Run(fmt.Sprintf("len%d/offset%d/c%d", n, offset, coef), func(t *testing.T) {
					srcBuf := bytes.Repeat([]byte{0xa7}, n+offset+9)
					dstBuf := bytes.Repeat([]byte{0x5d}, n+offset+9)
					for i := 0; i < n; i++ {
						srcBuf[offset+i] = byte(i*17 + n*3 + int(coef))
						dstBuf[offset+i] = byte(i*29 + 73)
					}
					src := srcBuf[offset : offset+n]
					dst := dstBuf[offset : offset+n]
					ref := append([]byte(nil), dst...)
					xorMulScalar(ref, src, coef)
					xorMul(dst, src, coef)
					if !bytes.Equal(dst, ref) {
						t.Fatal("SIMD/scalar differ")
					}
					if !bytes.Equal(srcBuf[:offset], bytes.Repeat([]byte{0xa7}, offset)) || !bytes.Equal(srcBuf[offset+n:], bytes.Repeat([]byte{0xa7}, 9)) {
						t.Fatal("input sentinel modified")
					}
					if !bytes.Equal(dstBuf[:offset], bytes.Repeat([]byte{0x5d}, offset)) || !bytes.Equal(dstBuf[offset+n:], bytes.Repeat([]byte{0x5d}, 9)) {
						t.Fatal("output sentinel modified")
					}
				})
			}
		}
	}
}

func BenchmarkFECMulNetworkSpans(b *testing.B) {
	fastMulOnce.Do(initFastMul)
	for _, n := range []int{32, 96, 256, 512, 1000, 1400} {
		in, out := make([]byte,n),make([]byte,n)
		for i := range in { in[i]=byte(i*13+7) }
		b.Run(fmt.Sprintf("auto/%d",n),func(b *testing.B){
			b.SetBytes(int64(n));b.ReportAllocs();b.ResetTimer()
			for i:=0;i<b.N;i++ {xorMul(out,in,37)}
		})
		b.Run(fmt.Sprintf("scalar/%d",n),func(b *testing.B){
			b.SetBytes(int64(n));b.ReportAllocs();b.ResetTimer()
			for i:=0;i<b.N;i++ {xorMulScalar(out,in,37)}
		})
	}
}
