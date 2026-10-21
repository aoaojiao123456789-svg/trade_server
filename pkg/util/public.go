package util

import (
	"crypto/sha256"
	"encoding/binary"
)

const (
	mod     = 1_000_000_000 // 9 位
	rounds  = 4
	halfMod = 100_000 // 拆成 5 位 + 4 位
)

var secret = []byte("asd45asdasd23sdbn") // 一定要保密

func feistelRound(l, r uint32, round int) (uint32, uint32) {
	h := sha256.New()
	h.Write(secret)
	h.Write([]byte{byte(round)})
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, r)
	h.Write(b)

	sum := h.Sum(nil)
	f := binary.BigEndian.Uint32(sum[:4]) % halfMod

	return r, (l + f) % halfMod
}

// Permute 把一个递增ID 映射成 9 位随机 UID
func Gen9UID(id uint32) int64 {
	l := id / halfMod
	r := id % halfMod

	for i := 0; i < rounds; i++ {
		l, r = feistelRound(l, r, i)
	}

	return int64((l*halfMod + r) % mod)
}
