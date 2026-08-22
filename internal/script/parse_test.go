package script

import (
	"strings"
	"testing"

	"task160-migimpact/internal/model"
)

func TestParseScript(t *testing.T) {
	content := `# 迁移注释
1 CREATE_TABLE [users]
2 ADD_COLUMN [column:users:email]
3 DROP_TABLE [users] # 行内注释
`
	specs, err := ParseScript(content)
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("期望 3 个步骤，得到 %d", len(specs))
	}
	if specs[0].Seq != 1 || specs[1].Seq != 2 || specs[2].Seq != 3 {
		t.Fatalf("序号不正确: %+v", specs)
	}
	if specs[2].TargetObj != "users" {
		t.Fatalf("目标对象应为 users，得到 %q", specs[2].TargetObj)
	}
}

func TestParseScriptNonContiguous(t *testing.T) {
	content := "1 CREATE_TABLE [users]\n3 DROP_TABLE [users]\n"
	_, err := ParseScript(content)
	if err == nil || !strings.Contains(err.Error(), "序号不连续") {
		t.Fatalf("应报序号不连续错误，得到 %v", err)
	}
}

func TestParseScriptUnknownType(t *testing.T) {
	_, err := ParseScript("1 FLY_TO_MOON [x]\n")
	if err == nil || !strings.Contains(err.Error(), "未知步骤类型") {
		t.Fatalf("应报未知步骤类型错误，得到 %v", err)
	}
}

func TestFingerprintStable(t *testing.T) {
	a := Fingerprint("1 DROP_TABLE [orders]")
	b := Fingerprint("1 DROP_TABLE [orders]")
	if a != b {
		t.Fatalf("相同内容指纹应一致: %s != %s", a, b)
	}
}

func TestVerifyVersionContinuity(t *testing.T) {
	existing := []model.MigrationScript{
		{Version: 1}, {Version: 2},
	}
	if err := VerifyVersionContinuity(existing, 3); err != nil {
		t.Fatalf("version 3 应通过: %v", err)
	}
	if err := VerifyVersionContinuity(existing, 2); err == nil {
		t.Fatal("重复版本应报错")
	}
	if err := VerifyVersionContinuity(nil, 1); err != nil {
		t.Fatalf("首个脚本版本 1 应通过: %v", err)
	}
	if err := VerifyVersionContinuity(nil, 2); err == nil {
		t.Fatal("首个脚本版本 2 应报错")
	}
}

func TestNormalizeTarget(t *testing.T) {
	if got := NormalizeTarget("orders"); got != "table:orders" {
		t.Fatalf("NormalizeTarget(orders) = %q", got)
	}
	if got := NormalizeTarget("column:orders:id"); got != "column:orders:id" {
		t.Fatalf("NormalizeTarget(column) = %q", got)
	}
}
