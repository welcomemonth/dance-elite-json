package utils

import (
	"strings"
	"testing"
)

// TestHashIDNumberNormalizes 同一身份证号的不同输入形式必须得到同一哈希，
// 否则同一人会被重复建档。
func TestHashIDNumberNormalizes(t *testing.T) {
	key := []byte("test-hmac-key")
	want := HashIDNumber(key, IDTypeIDCard, "11010119900307123X")

	variants := []string{
		"11010119900307123x",     // 末位小写
		" 11010119900307123X ",   // 首尾空格
		"11010119900307 123X",    // 中间空格
		"1101 0119 9003 0712 3X", // 分组空格
		"11010119900307123X\t",   // 制表符
		"　11010119900307123X",    // 全角空格
	}
	for _, in := range variants {
		if got := HashIDNumber(key, IDTypeIDCard, in); got != want {
			t.Errorf("HashIDNumber(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestHashIDNumberPassportCaseInsensitive(t *testing.T) {
	key := []byte("test-hmac-key")
	want := HashIDNumber(key, IDTypePassport, "E12345678")
	if got := HashIDNumber(key, IDTypePassport, "  e12345678 "); got != want {
		t.Errorf("passport hash = %s, want %s", got, want)
	}
}

func TestHashIDNumberDependsOnKey(t *testing.T) {
	a := HashIDNumber([]byte("key-a"), IDTypeIDCard, "11010119900307123X")
	b := HashIDNumber([]byte("key-b"), IDTypeIDCard, "11010119900307123X")
	if a == b {
		t.Error("different keys produced the same hash")
	}
}

func TestEqualIDHash(t *testing.T) {
	h := HashIDNumber([]byte("k"), IDTypeIDCard, "11010119900307123X")
	if !EqualIDHash(h, h) {
		t.Error("EqualIDHash should report identical hashes equal")
	}
	if EqualIDHash(h, HashIDNumber([]byte("k"), IDTypeIDCard, "11010119900307124X")) {
		t.Error("EqualIDHash should report different hashes unequal")
	}
}

func TestMaskIDNumber(t *testing.T) {
	got := MaskIDNumber("11010119900307123X")
	if got != "110***********123X" {
		t.Errorf("MaskIDNumber = %q, want %q", got, "110***********123X")
	}
	if strings.Contains(got, "19900307") {
		t.Errorf("mask leaked birth date: %q", got)
	}
	if short := MaskIDNumber("123"); short != "***" {
		t.Errorf("MaskIDNumber(short) = %q, want ***", short)
	}
	if empty := MaskIDNumber("  "); empty != "" {
		t.Errorf("MaskIDNumber(blank) = %q, want empty", empty)
	}
}
