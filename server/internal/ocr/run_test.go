package ocr

import "testing"

func TestBuildArgsReviewRequiresFrom(t *testing.T) {
	if _, err := buildArgs(RunSpec{Mode: "review"}, "out.json"); err == nil {
		t.Fatal("review 缺 from 应失败")
	}
	args, err := buildArgs(RunSpec{Mode: "review", From: "main", To: "feature"}, "out.json")
	if err != nil {
		t.Fatal(err)
	}
	got := join(args)
	if got != "review --from main --format json --output out.json --to feature" {
		t.Fatalf("args=%s", got)
	}
}

func TestBuildArgsScanRequiresPathUnlessAll(t *testing.T) {
	if _, err := buildArgs(RunSpec{Mode: "scan"}, "out.json"); err == nil {
		t.Fatal("scan 缺 path 应失败")
	}
	args, err := buildArgs(RunSpec{Mode: "scan", Path: "internal/"}, "out.json")
	if err != nil {
		t.Fatal(err)
	}
	if join(args) != "scan --path internal/ --format json --output out.json" {
		t.Fatalf("args=%v", args)
	}
	args, err = buildArgs(RunSpec{Mode: "scan", ScanAll: true}, "out.json")
	if err != nil {
		t.Fatal(err)
	}
	if join(args) != "scan --format json --output out.json" {
		t.Fatalf("scan all args=%v", args)
	}
}

func join(args []string) string {
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}
