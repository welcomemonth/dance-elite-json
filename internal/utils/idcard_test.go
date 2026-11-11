package utils

import (
	"errors"
	"strings"
	"testing"
)

// 手工核算的合法身份证向量（GB 11643-1999 加权校验码），钉死算法正确性；
// 表驱动的其他用例由 makeIDCard 生成，避免逐个手算。
var validIDCards = []struct {
	raw        string
	normalized string
	birthDate  string
	gender     string
}{
	{"110105201603070010", "110105201603070010", "2016-03-07", "male"},
	{"110105201501020020", "110105201501020020", "2015-01-02", "female"},
	// 末位小写 x（校验位恰为 X）+ 含空格，规范化后应与向量一致
	{"11010520160307007x", "11010520160307007X", "2016-03-07", "male"},
	{" 110105201501020020 ", "110105201501020020", "2015-01-02", "female"},
}

// makeIDCard 由 17 位本体码生成合法身份证（测试数据构造用；
// 算法正确性由上方手工核算向量保证）。
func makeIDCard(body17 string) string {
	sum := 0
	for i := range 17 {
		sum += int(body17[i]-'0') * idCardWeights[i]
	}
	return body17 + string(idCardCheckCodes[sum%11])
}

func TestParseIDCard_Valid(t *testing.T) {
	for _, tc := range validIDCards {
		normalized, birth, gender, err := ParseIDCard(tc.raw)
		if err != nil {
			t.Errorf("ParseIDCard(%q) unexpected error: %v", tc.raw, err)
			continue
		}
		if normalized != tc.normalized || birth != tc.birthDate || gender != tc.gender {
			t.Errorf("ParseIDCard(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.raw, normalized, birth, gender, tc.normalized, tc.birthDate, tc.gender)
		}
	}
}

func TestParseIDCard_BirthYearBoundaries(t *testing.T) {
	// 覆盖组别判定关心的出生年份边界：2012（U15 下限）与 2018（U11 上限）。
	for _, year := range []string{"2012", "2018"} {
		id := makeIDCard("110105" + year + "0701001")
		_, birth, _, err := ParseIDCard(id)
		if err != nil {
			t.Fatalf("ParseIDCard(%q): %v", id, err)
		}
		if want := year + "-07-01"; birth != want {
			t.Errorf("birth = %q, want %q", birth, want)
		}
	}
}

func TestParseIDCard_Invalid(t *testing.T) {
	cases := map[string]string{
		"长度不足":      "11010520160307001",
		"长度超长":      "1101052016030700111",
		"前 17 位含字母": "1101052016030700A",
		"末位非法字符":    "11010520160307001Y",
		"校验码错误":     "110105201603070012",            // 正确末位为 0
		"出生日期不存在":   makeIDCard("11010520150230001"), // 2 月 30 日
		"出生日期在未来":   makeIDCard("11010520990101001"),
		"空串":        "",
	}
	for name, raw := range cases {
		if _, _, _, err := ParseIDCard(raw); !errors.Is(err, ErrInvalidIDCard) {
			t.Errorf("%s: ParseIDCard(%q) err = %v, want ErrInvalidIDCard", name, raw, err)
		}
	}
}

func TestParseIDCard_ErrorDoesNotLeakNumber(t *testing.T) {
	raw := "110105201603070012"
	_, _, _, err := ParseIDCard(raw)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), raw) {
		t.Errorf("error message leaks id number: %v", err)
	}
}

func TestMakeIDCard_Sanity(t *testing.T) {
	// 生成器自洽：生成的号码必须能通过校验（防测试数据本身构造错误）。
	id := makeIDCard("11010520160307001")
	if id != "110105201603070010" {
		t.Fatalf("makeIDCard = %q, want hand-computed 110105201603070010", id)
	}
}
