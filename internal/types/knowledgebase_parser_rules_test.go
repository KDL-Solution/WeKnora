package types

import "testing"

func TestDefaultDeepOfficeParserEngineRules(t *testing.T) {
	rules := DefaultDeepOfficeParserEngineRules()
	if len(rules) != 2 {
		t.Fatalf("len(DefaultDeepOfficeParserEngineRules()) = %d, want 2", len(rules))
	}

	deepParserTypes := map[string]bool{}
	for _, ft := range rules[0].FileTypes {
		deepParserTypes[ft] = true
	}
	for _, ft := range []string{"pdf", "docx", "pptx", "hwp", "png", "gif"} {
		if !deepParserTypes[ft] {
			t.Fatalf("DeepParser default rule missing %q", ft)
		}
	}
	if deepParserTypes["odt"] {
		t.Fatalf("DeepParser default rule should not include odt")
	}
	if rules[0].Engine != ParserEngineDeepParser {
		t.Fatalf("DeepParser rule engine = %q", rules[0].Engine)
	}

	builtinTypes := map[string]bool{}
	for _, ft := range rules[1].FileTypes {
		builtinTypes[ft] = true
	}
	for _, ft := range []string{"md", "xlsx", "xls", "csv", "txt"} {
		if !builtinTypes[ft] {
			t.Fatalf("builtin default rule missing %q", ft)
		}
	}
	if rules[1].Engine != ParserEngineBuiltin {
		t.Fatalf("builtin rule engine = %q", rules[1].Engine)
	}
}

func TestResolveParserEngineKeepsBuiltinDocReaderExceptions(t *testing.T) {
	cfg := ChunkingConfig{
		ParserEngineRules: []ParserEngineRule{
			{FileTypes: []string{"pdf", "xlsx", "csv", "txt"}, Engine: ParserEngineDeepParser},
		},
	}

	if got := cfg.ResolveParserEngine("pdf"); got != ParserEngineDeepParser {
		t.Fatalf("ResolveParserEngine(pdf) = %q, want %q", got, ParserEngineDeepParser)
	}
	for _, ft := range []string{"xlsx", ".csv", "TXT"} {
		if got := cfg.ResolveParserEngine(ft); got != ParserEngineBuiltin {
			t.Fatalf("ResolveParserEngine(%q) = %q, want %q", ft, got, ParserEngineBuiltin)
		}
	}
}
