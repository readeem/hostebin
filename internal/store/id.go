package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
)

const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

func NewID() (string, error) {
	bits := 128
	if raw := os.Getenv("HOSTEBIN_ID_BITS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 64 || n > 256 {
			return "", errors.New("HOSTEBIN_ID_BITS must be an integer from 64 to 256")
		}
		bits = n
	}
	nbytes := (bits + 7) / 8
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random bundle id: %w", err)
	}
	if excess := nbytes*8 - bits; excess > 0 {
		b[0] &= byte(0xff >> excess)
	}
	n := new(big.Int).SetBytes(b)
	chars := (bits + 4) / 5
	out := make([]byte, chars)
	mask := big.NewInt(31)
	for i := chars - 1; i >= 0; i-- {
		v := new(big.Int).And(n, mask)
		out[i] = crockford[v.Int64()]
		n.Rsh(n, 5)
	}
	return string(out), nil
}
