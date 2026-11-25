package glossary

import "testing"

func TestMatchAndMissing(t *testing.T) {
	g, err := New([]Term{
		{Source: "Gus", Target: "ガス"},
		{Source: "Pier 9", Target: "第9埠頭"},
		{Source: "Vale", Target: "ヴェイル", CaseSensitive: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := g.Match("Ask gus about Pier 9, Vale. Not in August.")
	if len(got) != 3 {
		t.Fatalf("3 件一致のはずが %d 件: %+v", len(got), got)
	}
	if n := len(g.Match("It happened in August.")); n != 0 {
		t.Fatalf("単語の途中に一致してはいけない: %d 件", n)
	}
	if n := len(g.Match("a vale of tears")); n != 0 {
		t.Fatalf("case_sensitive の語が小文字に一致した")
	}

	miss := Missing("ガスに第9埠頭のことを聞け。", got)
	if len(miss) != 1 || miss[0].Source != "Vale" {
		t.Fatalf("Vale だけが不足のはずが %+v", miss)
	}
}

func TestApplyAliases(t *testing.T) {
	terms := []Term{
		{Source: "Dunn", Target: "Dunn", Aliases: []string{"ダン"}},
		{Source: "Holmes", Target: "Holmes", Aliases: []string{"ホームズ", "シャーロック・ホームズ"}},
	}
	got := ApplyAliases("ダンさんはダンスが好きだ。シャーロック・ホームズより。", terms)
	if got != "Dunnさんはダンスが好きだ。Holmesより。" {
		t.Fatalf("置き換えが違う: %q", got)
	}
	// 訳語がすでにあれば触らない
	if got := ApplyAliases("Dunnとダン", terms[:1]); got != "Dunnとダン" {
		t.Fatalf("訳語がある行は触らないはず: %q", got)
	}
}

func TestEmptyTermRejected(t *testing.T) {
	if _, err := New([]Term{{Source: "x"}}); err == nil {
		t.Fatal("target が空ならエラーになるべき")
	}
}
