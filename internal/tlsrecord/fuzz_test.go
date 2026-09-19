package tlsrecord

import "testing"

func FuzzDecoderOpenPayload(f *testing.F) {
	keys := testKeys(f)
	sealer, err := NewSealer(keys, MaxWireLen)
	if err != nil {
		f.Fatal(err)
	}
	valid, _, err := sealer.Seal([]byte("seed"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte{})
	f.Add([]byte{0x17, 0x03, 0x03, 0x00, 0x1a})

	f.Fuzz(func(t *testing.T, data []byte) {
		decoder, err := NewDecoder(keys, MaxWireLen)
		if err != nil {
			t.Fatal(err)
		}
		_ = decoder.OpenPayload(data)
	})
}
