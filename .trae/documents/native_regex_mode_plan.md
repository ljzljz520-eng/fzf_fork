# 原生正则模式（--regex）实现计划

## 背景与研究结论

fzf 扩展搜索模式（extended-search mode）目前只支持有限的匹配语法：模糊匹配、`'` 精确匹配、`^`/`$` 锚定、`!` 取反、` | ` OR。需要完整正则时用户只能管道外部命令（如 `... | grep -E 'pat' | fzf`）。

代码调研结论：

- 匹配算法统一为 `algo.Algo` 签名（[algo.go:322](file:///Users/kkcarrot/swe-project/fzf_fork/src/algo/algo.go#L322)）：
  `func(caseSensitive, normalize, forward bool, input *util.Chars, pattern []rune, withPos bool, slab *util.Slab) (Result, *[]int)`。
  现有 `ExactMatchNaive`、`PrefixMatch` 等都是该签名，新增正则算法可以无缝接入。
- 词项类型 `termType` 共 6 种（[pattern.go:23-30](file:///Users/kkcarrot/swe-project/fzf_fork/src/pattern.go#L23-L30)），`Pattern.procFun [6]algo.Algo` 按类型分派；扩展模式词项由 `parseTerms` 解析（[pattern.go:168](file:///Users/kkcarrot/swe-project/fzf_fork/src/pattern.go#L168)）。
- 关键约束：`parseTerms` 在大小写不敏感时会把词项**整体小写化**（pattern.go:182-184），这会破坏正则语法（`\D`→`\d`、`\S`→`\s`、`\W`→`\w`、`\B`→`\b`、`[A-Z]`→`[a-z]`）。因此正则词项**不能小写化**，大小写不敏感须改用 Go regexp 的 `(?i)` 内联标志。
- Go 的 `regexp` 为 RE2 语法，无回溯、无先行断言/反向引用，适合 fzf 的性能要求；`regexp.Compile` 每次调用都重新编译，必须按词项缓存编译结果。
- 偏移量约定：`util.Chars` 字节模式（纯 ASCII）下标即字节下标；rune 模式下标为 rune 下标。regexp 返回字节偏移，rune 模式下需用 `utf8.RuneCountInString` 转换。
- 归一化（normalize，默认开启，`--literal` 关闭）是 rune→rune 的 1:1 映射（[normalize.go](file:///Users/kkcarrot/swe-project/fzf_fork/src/algo/normalize.go)），不改变长度，可安全用于正则模式：词项 rune 归一化、匹配时输入 rune 归一化后再转字符串。
- 快速路径天然排除正则词项：`buildDirectAlgo` 只接受 `termFuzzy`（pattern.go:283-294）；位图缓存条件（pattern.go:115）对非 fuzzy/exact 词项自动置 `cacheable=false`；`buildCacheKey` 也只收 fuzzy/exact 词项。无需改动缓存逻辑。
- `!` 不是正则元字符，保留为取反前缀（RE2 无法表达否定匹配，取反仍有价值）；空格分隔 AND、独立 ` | ` 词项 OR、`\ ` 转义空格、`--nth`/`-d` 字段限定全部沿用现有解析，天然可用。
- 查询输入框不按词项类型着色（整段用 `ColInput`，terminal.go:3439-3446），无需 TUI 改动。
- man 页 `man/man1/fzf.1` 通过 `go:embed` 内置于 `--man`（main.go:41），改一处即可。

## 语义设计

新增 `--regex` / `--no-regex` 开关（无短选项，与 `--literal`、`--smart-case` 风格一致）。

开启后：

1. 扩展模式（默认）下，每个空格分隔的词项按 **Go RE2 完整正则**编译匹配；空格仍为 AND，独立 ` | ` 仍为 OR，词项内无空格的 `|` 为正则交替；`\ ` 仍表示字面空格。
2. 词项前缀 `!` 仍表示取反；`'`、`^`、`$` 不再解释为 fzf 操作符——`^`/`$` 即正则锚点（原生覆盖前缀/后缀/整行匹配），`'` 为普通字符。
3. 大小写：仍按 smart-case 逐词项判定（词项含大写→敏感）；不敏感时以 `(?i)` 编译，词项文本不小写化，保证 `\D` 等语法不被破坏。
4. 归一化：沿用现有逐词项判定；开启时词项 rune 与输入 rune 都做 latin 归一化。
5. 非扩展模式（`+x`）下 `--regex`：整条查询作为一个正则（空格为字面量）。
6. 正则非法时（如输入过程中的 `[`），词项编译失败→该词项不匹配任何行（不崩溃）；取反词项此时视为匹配（与"不满足条件"一致）。
7. `--filter` 非交互模式同样生效，可直接替代 `grep -E` 管道。
8. 评分：正则匹配不适用模糊打分，采用 `scoreMatch + 词边界奖励×首字符倍率` 的确定性分数；排序主要由 tiebreak（默认 length）决定。

## 修改文件

- `src/algo/algo.go`：新增 `RegexMatch` 算法函数 + 编译结果缓存（`sync.Map`）。
- `src/pattern.go`：新增 `termRegex` 词项类型；`Pattern` 增加 `regex` 字段；`BuildPattern`/`parseTerms` 增加 `regex` 参数；正则词项跳过小写化与 `'^$` 操作符解析；`basicMatch` 在 regex 模式走 `RegexMatch`；`procFun` 数组扩为 `[7]`。
- `src/options.go`：`Options` 增加 `Regex bool`；解析 `--regex`/`--no-regex`；Usage 文本增加说明。
- `src/core.go`：`BuildPattern` 调用传入 `opts.Regex`。
- `src/pattern_test.go`：更新 `buildPattern`/`parseTerms` 测试助手签名；新增正则解析与匹配测试。
- `src/options_test.go`：新增 `--regex` 标志解析测试。
- `man/man1/fzf.1`：OPTIONS 增加 `--regex` 条目；EXTENDED SEARCH MODE 章节增加正则模式说明。
- `README.md`：搜索语法章节补充 `--regex` 说明。
- `test/test_filter.rb`：新增少量 `--regex --filter` 集成测试（若 Ruby 环境可用）。

## 实现步骤

1. `src/algo/algo.go`
   - 增加 `sync` 导入与包级 `regexCache sync.Map`（key 含大小写标志 + 模式字符串）。
   - 新增 `compileRegex(caseSensitive bool, pattern []rune) *regexp.Regexp`：不敏感时加 `(?i)` 前缀；编译失败缓存类型化 nil 并返回 nil。
   - 新增 `RegexMatch`（Algo 签名）：空模式→`Result{0,0,0}`；编译失败→`Result{-1,-1,0}`；取输入字符串（normalize 时由 `NormalizeRunes(input.ToRunes())` 构造）；`FindStringIndex` 取最左匹配；rune 模式或归一化时用 `utf8.RuneCountInString` 把字节偏移转 rune 偏移；按 `forward=false` 做坐标翻转（与其他算法一致）；分数 `scoreMatch + bonusAt(input,sidx)*bonusFirstCharMultiplier`；pos 返回 nil（扩展匹配层会自动用整个匹配区间高亮）。
2. `src/pattern.go`
   - `termType` 末尾加 `termRegex`；`procFun [7]algo.Algo`；`ptr.procFun[termRegex] = algo.RegexMatch`。
   - `Pattern` 加 `regex bool` 字段；`BuildPattern` 加 `regex bool` 参数（置于 `extended` 之后）并赋值。
   - 非扩展分支：`!caseSensitive && !regex` 时才小写化 `asString`。
   - `parseTerms` 加 `regex bool` 参数：regex 时词项类型直接为 `termRegex`；`!` 取反不强制改 `termExact`；跳过 `$`/`'`/`^` 操作符处理段；不做 `text = lowerText` 小写化（caseSensitive 判定仍基于原文）；归一化逻辑不变。
   - `basicMatch`：`p.regex` 时用 `algo.RegexMatch`。
3. `src/options.go`
   - `Options` 结构体 `Extended` 附近加 `Regex bool`（零值 false 即默认关闭）。
   - SEARCH 段加 case `"--regex": opts.Regex = true` / `"--no-regex": opts.Regex = false`。
   - Usage 常量 SEARCH 段加 `    --regex              Treat search terms as regular expressions (RE2 syntax)`。
4. `src/core.go`：patternBuilder 的 `BuildPattern` 调用加 `opts.Regex`。
5. 测试：
   - `pattern_test.go`：更新两处 `BuildPattern` 调用与三处 `parseTerms` 调用的签名；新增 `TestParseTermsRegex`（类型为 termRegex、`^$'` 保留、`!` 取反、无小写化）、`TestRegexMatch`（`foo.*bar` 命中/不命中、`(?i)` 风格 smart-case、`^...$` 锚点、`!` 取反、` | ` OR、非法正则不匹配、非扩展模式整段正则、归一化）。
   - `options_test.go`：`optsFor("--regex").Regex == true`、`--no-regex` 为 false。
   - `test/test_filter.rb`：`--regex --filter 'foo.*bar'`、取反、大小写不敏感等用例。
6. 文档：`man/man1/fzf.1` 加选项条目与正则模式小节（说明 RE2 限制：无先行断言/反向引用；非法正则无匹配；` | ` 与词项内 `|` 的区别）；`README.md` 搜索语法处补充。

## 依赖与注意事项

- 仅用标准库 `regexp`、`sync`、`unicode/utf8`（algo.go 已导入 utf8），无新外部依赖。
- RE2 语义：不支持 lookahead/lookbehind/反向引用，文档中说明。
- 正则匹配比模糊匹配慢（无字符预过滤）；编译结果缓存保证同一词项只编译一次；regexp 本身为线性时间，无 ReDoS 风险。
- `--algo`、`-e/--exact` 在正则模式下对正则词项无作用（模糊/精确算法不被调用），不报错、不互斥。
- `--nth` 字段限定、ANSI 处理、`--read0` 多行（`.` 默认不匹配换行，可用 `(?s)`）均自然兼容。

## 验证

- `go build ./...` 编译通过。
- `go test ./src/...`（含 fzf 包与 algo 包）全部通过。
- 手工验证：
  - `printf 'foo\nfoobar\nbar\n' | go run . --regex --filter 'foo.*bar'` → 仅 `foobar`；
  - `... --regex --filter '^FOO'`（smart-case 敏感）无结果，`-i` 命中；
  - `... --regex --filter '!bar'` 取反；
  - `... --regex --filter 'foo|bar'`（词项内交替）与 `'foo | bar'`（OR）语义差异；
  - `printf 'a1\na2\nb1\n' | go run . --regex --filter 'a[0-9]'`；
  - 非法正则 `[` 不崩溃、无输出。
- 若 Ruby/bundler 可用，运行 `test/test_filter.rb` 中新用例。

## 风险

- 非法正则输入中无任何提示（仅无匹配）：与 grep 行为类似，保持最小实现；后续可在 info 行加错误提示。
- 正则模式评分较简单：排序由 tiebreak 兜底，行为确定可预期。
- 取反词项遇非法正则视为"条件不满足"而全部通过：边缘情况，文档说明。
