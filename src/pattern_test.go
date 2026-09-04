package fzf

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

var slab *util.Slab

func init() {
	slab = util.MakeSlab(slab16Size, slab32Size)
}

func TestParseTermsExtended(t *testing.T) {
	terms := parseTerms(true, false, CaseSmart, false,
		"aaa 'bbb ^ccc ddd$ !eee !'fff !^ggg !hhh$ | ^iii$ ^xxx | 'yyy | zzz$ | !ZZZ |")
	if len(terms) != 9 ||
		terms[0][0].typ != termFuzzy || terms[0][0].inv ||
		terms[1][0].typ != termExact || terms[1][0].inv ||
		terms[2][0].typ != termPrefix || terms[2][0].inv ||
		terms[3][0].typ != termSuffix || terms[3][0].inv ||
		terms[4][0].typ != termExact || !terms[4][0].inv ||
		terms[5][0].typ != termFuzzy || !terms[5][0].inv ||
		terms[6][0].typ != termPrefix || !terms[6][0].inv ||
		terms[7][0].typ != termSuffix || !terms[7][0].inv ||
		terms[7][1].typ != termEqual || terms[7][1].inv ||
		terms[8][0].typ != termPrefix || terms[8][0].inv ||
		terms[8][1].typ != termExact || terms[8][1].inv ||
		terms[8][2].typ != termSuffix || terms[8][2].inv ||
		terms[8][3].typ != termExact || !terms[8][3].inv {
		t.Errorf("%v", terms)
	}
	for _, termSet := range terms[:8] {
		term := termSet[0]
		if len(term.text) != 3 {
			t.Errorf("%v", term)
		}
	}
}

func TestParseTermsExtendedExact(t *testing.T) {
	terms := parseTerms(false, false, CaseSmart, false,
		"aaa 'bbb ^ccc ddd$ !eee !'fff !^ggg !hhh$")
	if len(terms) != 8 ||
		terms[0][0].typ != termExact || terms[0][0].inv || len(terms[0][0].text) != 3 ||
		terms[1][0].typ != termFuzzy || terms[1][0].inv || len(terms[1][0].text) != 3 ||
		terms[2][0].typ != termPrefix || terms[2][0].inv || len(terms[2][0].text) != 3 ||
		terms[3][0].typ != termSuffix || terms[3][0].inv || len(terms[3][0].text) != 3 ||
		terms[4][0].typ != termExact || !terms[4][0].inv || len(terms[4][0].text) != 3 ||
		terms[5][0].typ != termFuzzy || !terms[5][0].inv || len(terms[5][0].text) != 3 ||
		terms[6][0].typ != termPrefix || !terms[6][0].inv || len(terms[6][0].text) != 3 ||
		terms[7][0].typ != termSuffix || !terms[7][0].inv || len(terms[7][0].text) != 3 {
		t.Errorf("%v", terms)
	}
}

func TestParseTermsEmpty(t *testing.T) {
	terms := parseTerms(true, false, CaseSmart, false, "' ^ !' !^")
	if len(terms) != 0 {
		t.Errorf("%v", terms)
	}
}

func TestParseTermsRegex(t *testing.T) {
	terms := parseTerms(true, true, CaseSmart, false, `^abc def$ 'ghi !jkl | FOO bar\ baz`)
	// !jkl and FOO form one OR term set
	if len(terms) != 5 {
		t.Fatalf("5 term sets expected: %v", terms)
	}
	// ^, $, and ' are not interpreted as fzf term markers in regex mode
	if terms[0][0].typ != termRegex || terms[0][0].inv || string(terms[0][0].text) != "^abc" {
		t.Errorf("anchored regex term: %v", terms[0][0])
	}
	if terms[1][0].typ != termRegex || terms[1][0].inv || string(terms[1][0].text) != "def$" {
		t.Errorf("suffix regex term: %v", terms[1][0])
	}
	if terms[2][0].typ != termRegex || terms[2][0].inv || string(terms[2][0].text) != "'ghi" {
		t.Errorf("quoted regex term: %v", terms[2][0])
	}
	// ! still negates, and the rest is treated as a regular expression
	if terms[3][0].typ != termRegex || !terms[3][0].inv || string(terms[3][0].text) != "jkl" {
		t.Errorf("inverse regex term: %v", terms[3][0])
	}
	// A standalone | is still the OR operator; the uppercase term is kept
	// verbatim and smart-case marks it case-sensitive
	if terms[3][1].typ != termRegex || terms[3][1].inv || !terms[3][1].caseSensitive ||
		string(terms[3][1].text) != "FOO" {
		t.Errorf("OR regex term: %v", terms[3][1])
	}
	// Escaped space is a literal space inside the regular expression
	if terms[4][0].typ != termRegex || terms[4][0].inv || string(terms[4][0].text) != "bar baz" {
		t.Errorf("escaped-space regex term: %v", terms[4][0])
	}
}

func regexPattern(caseMode Case, normalize bool, extended bool, query string) *Pattern {
	return buildPattern(true, algo.FuzzyMatchV2, extended, true, caseMode, normalize, true, false, true,
		[]Range{}, Delimiter{}, []rune(query))
}

func TestRegexMatch(t *testing.T) {
	matches := func(pat *Pattern, str string) bool {
		item := Item{text: util.ToChars([]byte(str))}
		match, _, _ := pat.MatchItem(&item, false, slab)
		return match.item != nil
	}

	// Basic regular-expression matching
	pat := regexPattern(CaseSmart, false, true, `foo.*bar`)
	if !matches(pat, "foobar") || !matches(pat, "foo---bar") || matches(pat, "foo") || matches(pat, "bar") {
		t.Error("foo.*bar")
	}

	// ^ and $ are regular-expression anchors
	pat = regexPattern(CaseSmart, false, true, `^bar$`)
	if !matches(pat, "bar") || matches(pat, "barbaz") || matches(pat, "foobar") {
		t.Error("^bar$")
	}

	// In-term alternation and the fzf OR operator both work
	pat = regexPattern(CaseSmart, false, true, `foo|bar`)
	if !matches(pat, "foo") || !matches(pat, "bar") || matches(pat, "baz") {
		t.Error("foo|bar alternation")
	}
	pat = regexPattern(CaseSmart, false, true, `foo | bar`)
	if !matches(pat, "foo") || !matches(pat, "bar") || matches(pat, "baz") {
		t.Error("foo | bar OR")
	}

	// Negation
	pat = regexPattern(CaseSmart, false, true, `!bar`)
	if matches(pat, "bar") || !matches(pat, "foo") {
		t.Error("!bar negation")
	}

	// Smart-case: an uppercase term is case-sensitive, a lowercase one is not
	pat = regexPattern(CaseSmart, false, true, `FOO`)
	if matches(pat, "foobar") || !matches(pat, "FOOBAR") {
		t.Error("smart-case sensitive term")
	}
	pat = regexPattern(CaseSmart, false, true, `foo`)
	if !matches(pat, "FOOBAR") {
		t.Error("smart-case insensitive term")
	}
	pat = regexPattern(CaseIgnore, false, true, `[A-Z]+`)
	if !matches(pat, "abc") {
		t.Error("ignore-case character class")
	}

	// Case-insensitive compilation must not lowercase the expression:
	// \D (non-digit) must not turn into \d (digit)
	chars := util.ToChars([]byte("abc"))
	res, _ := algo.RegexMatch(false, false, true, &chars, []rune(`\D`), false, nil)
	if res.Start < 0 {
		t.Error(`\D should match a non-digit under case-insensitive mode`)
	}

	// An invalid regular expression never matches
	pat = regexPattern(CaseSmart, false, true, `[`)
	if matches(pat, "anything") {
		t.Error("invalid regex should not match")
	}

	// Non-extended mode: the entire query is one regular expression
	pat = regexPattern(CaseSmart, false, false, `foo bar`)
	if !matches(pat, "foo bar") || matches(pat, "foobar") {
		t.Error("non-extended regex")
	}

	// Normalization: latin accented letters are folded
	pat = regexPattern(CaseSmart, true, true, `cafe`)
	if !matches(pat, "café") {
		t.Error("normalized regex match")
	}

	// Offsets are reported in rune indices for multi-byte items
	pat = regexPattern(CaseSmart, false, true, `foo.bar`)
	item := Item{text: util.ToChars([]byte("café foo-bar"))}
	match, offsets, _ := pat.MatchItem(&item, true, slab)
	if match.item == nil || offsets[0][0] != 5 || offsets[0][1] != 12 {
		t.Errorf("rune-space offsets: %v %v", match.item, offsets)
	}
}

func buildPattern(fuzzy bool, fuzzyAlgo algo.Algo, extended bool, regex bool, caseMode Case, normalize bool, forward bool,
	withPos bool, cacheable bool, nth []Range, delimiter Delimiter, runes []rune) *Pattern {
	return BuildPattern(NewChunkCache(), make(map[string]*Pattern),
		fuzzy, fuzzyAlgo, extended, regex, caseMode, normalize, forward,
		withPos, cacheable, nth, delimiter, revision{}, runes, nil, 0)
}

func TestExact(t *testing.T) {
	pattern := buildPattern(true, algo.FuzzyMatchV2, true, false, CaseSmart, false, true, false, true,
		[]Range{}, Delimiter{}, []rune("'abc"))
	chars := util.ToChars([]byte("aabbcc abc"))
	res, pos := algo.ExactMatchNaive(
		pattern.caseSensitive, pattern.normalize, pattern.forward, &chars, pattern.termSets[0][0].text, true, nil)
	if res.Start != 7 || res.End != 10 {
		t.Errorf("%v / %d / %d", pattern.termSets, res.Start, res.End)
	}
	if pos != nil {
		t.Errorf("pos is expected to be nil")
	}
}

func TestEqual(t *testing.T) {
	pattern := buildPattern(true, algo.FuzzyMatchV2, true, false, CaseSmart, false, true, false, true, []Range{}, Delimiter{}, []rune("^AbC$"))

	match := func(str string, sidxExpected int, eidxExpected int) {
		chars := util.ToChars([]byte(str))
		res, pos := algo.EqualMatch(
			pattern.caseSensitive, pattern.normalize, pattern.forward, &chars, pattern.termSets[0][0].text, true, nil)
		if res.Start != sidxExpected || res.End != eidxExpected {
			t.Errorf("%v / %d / %d", pattern.termSets, res.Start, res.End)
		}
		if pos != nil {
			t.Errorf("pos is expected to be nil")
		}
	}
	match("ABC", -1, -1)
	match("AbC", 0, 3)
	match("AbC  ", 0, 3)
	match(" AbC ", 1, 4)
	match("  AbC", 2, 5)
}

func TestCaseSensitivity(t *testing.T) {
	pat1 := buildPattern(true, algo.FuzzyMatchV2, false, false, CaseSmart, false, true, false, true, []Range{}, Delimiter{}, []rune("abc"))
	pat2 := buildPattern(true, algo.FuzzyMatchV2, false, false, CaseSmart, false, true, false, true, []Range{}, Delimiter{}, []rune("Abc"))
	pat3 := buildPattern(true, algo.FuzzyMatchV2, false, false, CaseIgnore, false, true, false, true, []Range{}, Delimiter{}, []rune("abc"))
	pat4 := buildPattern(true, algo.FuzzyMatchV2, false, false, CaseIgnore, false, true, false, true, []Range{}, Delimiter{}, []rune("Abc"))
	pat5 := buildPattern(true, algo.FuzzyMatchV2, false, false, CaseRespect, false, true, false, true, []Range{}, Delimiter{}, []rune("abc"))
	pat6 := buildPattern(true, algo.FuzzyMatchV2, false, false, CaseRespect, false, true, false, true, []Range{}, Delimiter{}, []rune("Abc"))

	if string(pat1.text) != "abc" || pat1.caseSensitive != false ||
		string(pat2.text) != "Abc" || pat2.caseSensitive != true ||
		string(pat3.text) != "abc" || pat3.caseSensitive != false ||
		string(pat4.text) != "abc" || pat4.caseSensitive != false ||
		string(pat5.text) != "abc" || pat5.caseSensitive != true ||
		string(pat6.text) != "Abc" || pat6.caseSensitive != true {
		t.Error("Invalid case conversion")
	}
}

func TestOrigTextAndTransformed(t *testing.T) {
	pattern := buildPattern(true, algo.FuzzyMatchV2, true, false, CaseSmart, false, true, false, true, []Range{}, Delimiter{}, []rune("jg"))
	tokens := Tokenize("junegunn", Delimiter{})
	trans := Transform(tokens, []Range{{1, 1}})

	origBytes := []byte("junegunn.choi")
	for _, extended := range []bool{false, true} {
		chunk := Chunk{count: 1}
		chunk.items[0] = Item{
			text:        util.ToChars([]byte("junegunn")),
			origText:    &origBytes,
			transformed: &transformed{pattern.revision, trans}}
		pattern.extended = extended
		matches, _ := pattern.matchChunk(&chunk, nil, slab) // No cache
		if !(matches[0].item.text.ToString() == "junegunn" &&
			string(*matches[0].item.origText) == "junegunn.choi" &&
			reflect.DeepEqual((*matches[0].item.transformed).tokens, trans)) {
			t.Error("Invalid match result", matches)
		}

		match, offsets, pos := pattern.MatchItem(&chunk.items[0], true, slab)
		if !(match.item.text.ToString() == "junegunn" &&
			string(*match.item.origText) == "junegunn.choi" &&
			offsets[0][0] == 0 && offsets[0][1] == 5 &&
			reflect.DeepEqual((*match.item.transformed).tokens, trans)) {
			t.Error("Invalid match result", match, offsets, extended)
		}
		if !((*pos)[0] == 4 && (*pos)[1] == 0) {
			t.Error("Invalid pos array", *pos)
		}
	}
}

func TestCacheKey(t *testing.T) {
	test := func(extended bool, patStr string, expected string, cacheable bool) {
		pat := buildPattern(true, algo.FuzzyMatchV2, extended, false, CaseSmart, false, true, false, true, []Range{}, Delimiter{}, []rune(patStr))
		if pat.CacheKey() != expected {
			t.Errorf("Expected: %s, actual: %s", expected, pat.CacheKey())
		}
		if pat.cacheable != cacheable {
			t.Errorf("Expected: %t, actual: %t (%s)", cacheable, pat.cacheable, patStr)
		}
	}
	test(false, "foo !bar", "foo !bar", true)
	test(false, "foo | bar !baz", "foo | bar !baz", true)
	test(true, "foo  bar  baz", "foo\tbar\tbaz", true)
	test(true, "foo !bar", "foo", false)
	test(true, "foo !bar   baz", "foo\tbaz", false)
	test(true, "foo | bar baz", "baz", false)
	test(true, "foo | bar | baz", "", false)
	test(true, "foo | bar !baz", "", false)
	test(true, "| | foo", "", false)
	test(true, "| | | foo", "foo", false)
}

func TestCacheable(t *testing.T) {
	test := func(fuzzy bool, str string, expected string, cacheable bool) {
		pat := buildPattern(fuzzy, algo.FuzzyMatchV2, true, false, CaseSmart, true, true, false, true, []Range{}, Delimiter{}, []rune(str))
		if pat.CacheKey() != expected {
			t.Errorf("Expected: %s, actual: %s", expected, pat.CacheKey())
		}
		if cacheable != pat.cacheable {
			t.Errorf("Invalid Pattern.cacheable for \"%s\": %v (expected: %v)", str, pat.cacheable, cacheable)
		}
	}
	test(true, "foo bar", "foo\tbar", true)
	test(true, "foo 'bar", "foo\tbar", false)
	test(true, "foo !bar", "foo", false)

	test(false, "foo bar", "foo\tbar", true)
	test(false, "foo 'bar", "foo", false)
	test(false, "foo '", "foo", true)
	test(false, "foo 'bar", "foo", false)
	test(false, "foo !bar", "foo", false)
}

func buildChunks(numChunks int) []*Chunk {
	chunks := make([]*Chunk, numChunks)
	words := []string{
		"src/main/java/com/example/service/UserService.java",
		"src/test/java/com/example/service/UserServiceTest.java",
		"docs/api/reference/endpoints.md",
		"lib/internal/utils/string_helper.go",
		"pkg/server/http/handler/auth.go",
		"build/output/release/app.exe",
		"config/production/database.yml",
		"scripts/deploy/kubernetes/setup.sh",
		"vendor/github.com/junegunn/fzf/src/core.go",
		"node_modules/.cache/babel/transform.js",
	}
	for ci := range numChunks {
		chunks[ci] = &Chunk{count: chunkSize}
		for i := range chunkSize {
			text := words[(ci*chunkSize+i)%len(words)]
			chunks[ci].items[i] = Item{text: util.ToChars([]byte(text))}
			chunks[ci].items[i].text.Index = int32(ci*chunkSize + i)
		}
	}
	return chunks
}

func buildPatternWith(cache *ChunkCache, runes []rune) *Pattern {
	return BuildPattern(cache, make(map[string]*Pattern),
		true, algo.FuzzyMatchV2, true, false, CaseSmart, false, true,
		false, true, []Range{}, Delimiter{}, revision{}, runes, nil, 0)
}

func TestBitmapCacheBenefit(t *testing.T) {
	numChunks := 100
	chunks := buildChunks(numChunks)
	queries := []string{"s", "se", "ser", "serv", "servi"}

	// 1. Run all queries with shared cache (simulates incremental typing)
	cache := NewChunkCache()
	for _, q := range queries {
		pat := buildPatternWith(cache, []rune(q))
		for _, chunk := range chunks {
			pat.Match(chunk, slab)
		}
	}

	// 2. GC and measure memory with cache populated
	runtime.GC()
	runtime.GC()
	var memWith runtime.MemStats
	runtime.ReadMemStats(&memWith)

	// 3. Clear cache, GC, measure again
	cache.Clear()
	runtime.GC()
	runtime.GC()
	var memWithout runtime.MemStats
	runtime.ReadMemStats(&memWithout)

	cacheMem := int64(memWith.Alloc) - int64(memWithout.Alloc)
	t.Logf("Chunks: %d, Queries: %d", numChunks, len(queries))
	t.Logf("Cache memory: %d bytes (%.1f KB)", cacheMem, float64(cacheMem)/1024)
	t.Logf("Per-chunk-per-query: %.0f bytes", float64(cacheMem)/float64(numChunks*len(queries)))

	// 4. Verify correctness: cached vs uncached produce same results
	cache2 := NewChunkCache()
	for _, q := range queries {
		pat := buildPatternWith(cache2, []rune(q))
		for _, chunk := range chunks {
			pat.Match(chunk, slab)
		}
	}
	for _, q := range queries {
		patCached := buildPatternWith(cache2, []rune(q))
		patFresh := buildPatternWith(NewChunkCache(), []rune(q))
		var countCached, countFresh int
		for _, chunk := range chunks {
			countCached += len(patCached.Match(chunk, slab))
			countFresh += len(patFresh.Match(chunk, slab))
		}
		if countCached != countFresh {
			t.Errorf("query=%q: cached=%d, fresh=%d", q, countCached, countFresh)
		}
		t.Logf("query=%q: matches=%d", q, countCached)
	}
}

func BenchmarkWithCache(b *testing.B) {
	numChunks := 100
	chunks := buildChunks(numChunks)
	queries := []string{"s", "se", "ser", "serv", "servi"}

	b.Run("cached", func(b *testing.B) {
		for range b.N {
			cache := NewChunkCache()
			for _, q := range queries {
				pat := buildPatternWith(cache, []rune(q))
				for _, chunk := range chunks {
					pat.Match(chunk, slab)
				}
			}
		}
	})

	b.Run("uncached", func(b *testing.B) {
		for range b.N {
			for _, q := range queries {
				cache := NewChunkCache()
				pat := buildPatternWith(cache, []rune(q))
				for _, chunk := range chunks {
					pat.Match(chunk, slab)
				}
			}
		}
	})
}
