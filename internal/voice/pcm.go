package voice

import "encoding/binary"

// int16ToLEBytes serializes 16-bit PCM samples to little-endian bytes, the wire
// format Vosk's AcceptWaveform expects (16 kHz, mono, signed 16-bit LE). Returns
// a buffer of exactly 2*len(samples) bytes.
func int16ToLEBytes(samples []int16) []byte {
	out := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(s))
	}
	return out
}
