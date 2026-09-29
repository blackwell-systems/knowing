[English](../../README.md) · [简体中文](README.zh-CN.md) · [Русский](README.ru.md) · [हिन्दी](README.hi.md) · **العربية**

<p align="center">
  <img src="../../assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
  <a href="https://zenodo.org/records/20342255"><img src="https://zenodo.org/badge/DOI/10.5281/zenodo.20342255.svg" alt="DOI"></a>
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-28%20tools%20%2B%208%20resources-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/languages_and_formats-26-blue.svg" alt="Languages and Formats"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/license-Apache_2.0-blue.svg" alt="License"></a>
</p>

---

محرك ذكاء برمجي ذاتي التكيّف. يراقب كثافة الرسم البياني الخاص به ويضبط استراتيجية الاسترجاع تلقائيًا. 38 نوعًا من الحواف، و28 أداة MCP، و263 فئة تكافؤ، وإثباتات تشفيرية. يزداد ذكاءً مع اتساع النطاق، لا غباءً.

---

> [!NOTE]
> **مبني على بحث منشور:** [Content-Addressing as a Computation Primitive for Software Relationship Intelligence](https://zenodo.org/records/20342255) (DOI: 10.5281/zenodo.20342255)

مخطط بنيتك المعمارية يقول إن الخدمة A تستدعي الخدمة B. هل يمكنك إثبات ذلك؟

**knowing يستطيع.** فهو يبني رسمًا بيانيًا معنونًا بالمحتوى (content-addressed) للعلاقات المستخرجة من الشيفرة، ويلتقط له لقطة على هيئة شجرة Merkle مرتبطة بـ git commit، ويولّد إثباتات تشفيرية يمكن التحقق منها دون اتصال. تستخدمه الوكلاء (Agents) للحصول على سياق مرتّب. وتستخدمه فرق الأمن للتدقيق. وتستخدمه فرق المنصّات لمقارنة الشيفرة بآثار الإنتاج (production traces).

يتحسّن في كل مرة تستخدمه فيها. وعندما تتغيّر الشيفرة، تنتهي صلاحية المعرفة القديمة تلقائيًا.

```bash
brew install blackwell-systems/tap/knowing
```

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

هذا كل شيء. يفهرس خادم MCP مستودعك تلقائيًا عند أول تشغيل. لا تنزيلات لنماذج، ولا مفاتيح API. أصبح لدى وكيلك الآن سياق مرتّب، ونطاق تأثير (blast radius)، ونطاق اختبار، وخفضٌ ضمني للضوضاء يحسّن النتائج أثناء الجلسات النشطة.

**تحقّق من أنه يعمل:** اطلب من وكيلك: *"استخدم أداة context_for_task للعثور على الرموز المرتبطة بـ [شيء تعرف أنه موجود في شيفرتك]."* ينبغي أن ترى رموزًا مرتّبة مع درجات ومسارات ملفات من قاعدة شيفرتك. إذا كانت النتائج فارغة، فإن المستودع لا يزال قيد الفهرسة (10-30 ثانية عند أول تشغيل). وإذا بدت النتائج غير ذات صلة، فراجع [استكشاف الأخطاء وإصلاحها](../../docs/guide/cli.md#troubleshooting).

> **لا تستخدم وكيل ذكاء اصطناعي؟** انتقل إلى [استخدام CLI](#path-b-cli-usage-explore-the-graph-yourself) أدناه.

| تريد أن... | ابدأ من هنا |
|---|---|
| تمنح وكيل الذكاء الاصطناعي سياقًا مرتّبًا بالرسم البياني | [إعداد MCP](#mcp-integration) |
| تستكشف الرسم البياني من CLI | [استخدام CLI](#path-b-cli-usage-explore-the-graph-yourself) |
| تفهم كيف يعمل الاسترجاع | [مقدمة](../../docs/guide/introduction.md) |
| تُدقّق بإثباتات تشفيرية | [التدقيق والامتثال](../../docs/guide/audit-compliance.md) |

---

## ثلاثة أشياء، بنية معمارية واحدة

knowing هو ثلاثة منتجات مبنية على أساس واحد (رسم بياني معنون بالمحتوى مع أشجار Merkle هرمية):

**1. محرك سياق لوكلاء الذكاء الاصطناعي**
استدعاء واحد يُرجع الرموز الأكثر صلة بمهمة ما، مرتّبةً حسب مركزية الرسم البياني والحداثة والفائدة المكتسبة، ومُحزَّمة لتناسب ميزانية الرموز (tokens) لديك. تسدّ 263 فئة تكافؤ للأُطر الفجوات المعجمية عندما تفشل الكلمات المفتاحية. عدد أقل من استدعاءات الأدوات بنسبة 47%. عدد أقل من الرموز بنسبة 84%. وتتحسّن النتائج مع التغذية الراجعة.

**2. بدائية تدقيق للامتثال**
كل حالة من حالات الرسم البياني هي جذر Merkle مرتبط بـ git commit. يولّد `knowing prove` إثباتًا تشفيريًا على أن علاقة ما كانت موجودة. ويتحقق `knowing verify` منه دون اتصال. ويتحقق `knowing fsck` من الرسم البياني بأكمله في 98 مللي ثانية. ويستخرج كشف سلسلة التوريد حواف الوصول إلى بيانات الاعتماد، وتوليد العمليات (process spawning)، والتسريب الشبكي (network exfiltration) لوسم الشيفرة المريبة بنيويًا.

**3. خفض ضوضاء يتعلّم**
الرموز التي أُرجعت لكن الوكيل لم يستخدمها قط تُخفَّض في الاستعلامات المستقبلية. وعندما تتغيّر الشيفرة، تنتهي صلاحية التغذية الراجعة تلقائيًا (يُتحقق منها عبر جذور Merkle للحزمة). يزداد النظام دقةً أثناء الجلسات النشطة. وهذه هي الخاصية التي بُني knowing حولها.

**هذه ليست ميزات منفصلة.** بل هي نتائج بنيوية للعَنْوَنة بالمحتوى: التجزئة (hash) نفسها التي تجعل السياق قابلاً للتخزين المؤقت تجعله كذلك قابلاً للإثبات، وجذر Merkle نفسه الذي يكشف القِدَم هو الذي يُنهي صلاحية التغذية الراجعة القديمة.

---

## ما الذي يجيب عنه

**لوكيلك:**
- "أنا أغيّر هذه الدالة. ما الذي سيتعطّل؟" (نطاق التأثير عبر المستدعين، والاختبارات، والمسارات، والمستودعات)
- "أعطني 50,000 رمز من السياق لهذه المهمة." (مرتّب بالرسم البياني، لا مبحوث بـ grep)
- "ما الاختبارات التي ينبغي تشغيلها؟" (اجتياز رسم الاستدعاء، بدقة 98%)

**لفريق المنصّات لديك:**
- "هل هذا المسار مُستخدَم في الإنتاج؟" (تحليل ساكن + آثار وقت التشغيل من OTel)
- "كيف كان يبدو رسم الخدمات عند لقطة معيّنة؟" (سلسلة لقطات، كل جذر مرتبط بـ git commit)

**لفريق الأمن لديك:**
- "أثبت أن الخدمة A تستدعي الخدمة B عند هذا الـ commit." (إثبات Merkle، قابل للتحقق دون اتصال)
- "أثبت أن هذه التبعية غير موجودة." (إثبات غياب عبر أوراق مُرتّبة)
- "ولّد تقرير امتثال." (`knowing audit -proofs`، أمر واحد)
- "هل تقرأ هذه الحزمة بيانات الاعتماد وتولّد عمليات؟" (`knowing audit-supply-chain --scan-all`)

---

## الأرقام

| ماذا | النتيجة |
|---|---:|
| الاسترجاع عبر الأنظمة | **P@10=0.330 بداية باردة** (302 مهمة، 17 مستودعًا، 8 لغات) |
| مقابل المنافسين | 3.79x codegraph (19K نجمة)، 6.00x GitNexus، 6.35x Gortex، 22.0x grep |
| فئات التكافؤ | 277 مُنتقاة يدويًا + مكتسبة من الاستخدام، تربط المعجم بالرموز (+57% P@10) |
| خفض الضوضاء | تغذية راجعة ضمنية لكل عنقود: R@10 +5.2%، MRR +12.6% (Django، 5 جولات) |
| استدعاءات أدوات مُوفَّرة | أقل بنسبة 47% (استدعاء سياق واحد يحل محل grep+read المتكرر) |
| توفير الرموز | أقل بنسبة 84% من الرموز (تنسيق الشبكة GCF) |
| سرعة الاستعلام المتكرر | أسرع بـ 93x (ذاكرة مؤقتة للرسم الفرعي بمفتاح Merkle) |
| فرق Merkle | أسرع بـ 517x من المسح الكامل للحواف عند 100K حافة |
| نطاق الاختبار | دقة 98%، استدعاء (recall) 82% |
| فحص سلامة الرسم البياني | 98 مللي ثانية (24,936 حافة) |
| توليد الإثبات | 72 ميكرو ثانية للتوليد، 1.2 ميكرو ثانية للتحقق |
| انتهاء صلاحية التغذية الراجعة | انتهاء 100% عند تغيّر الشيفرة، بحمل زائد 11% |
| إنتاجية الفهرسة | 16 مستودعًا (8 لغات) في ~60 ثانية |
| تغطية اللغات | 16/16 مستودعًا تنجح (Go، Python، TS، Rust، Java، C#، Ruby، متعدد) |
| أنواع الحواف | 38 (بما في ذلك سلسلة التوريد: reads_env، executes_process) |

جميع القياسات المرجعية قابلة لإعادة الإنتاج. يستخدم القياس المرجعي عبر الأنظمة (P@10=0.330) عددًا قدره 17 مستودعًا مثبّتة على commits دقيقة، مع [بيان جسم البيانات (corpus manifest)](../../bench/cross-system/corpus/MANIFEST.yaml) و[سكربت الإعداد](../../bench/cross-system/corpus/corpus-setup.sh) لإعادة إنتاج كاملة من الصفر. راجع [METHODOLOGY.md](../../bench/cross-system/METHODOLOGY.md) لتفاصيل البروتوكول.

---

## بداية سريعة

### المسار A: خادم MCP (مُوصى به لوكلاء الذكاء الاصطناعي)

```bash
# 1. Install
brew install blackwell-systems/tap/knowing
# Or: npm install -g @blackwell-systems/knowing
# Or: pip install knowing
# Or: go install github.com/blackwell-systems/knowing/cmd/knowing@latest

# 2. Add to your agent config (.mcp.json, Claude Code settings, etc.)
#    See "MCP Integration" below for the config block.
#    The server auto-indexes your repo on first launch. Done.
```

### المسار B: استخدام CLI (استكشف الرسم البياني بنفسك)

```bash
# 1. Install (same as above)
brew install blackwell-systems/tap/knowing

# 2. Index your repo
knowing add .

# 3. Verify the index worked
knowing stats
# You should see node and edge counts. A healthy TypeScript repo with 50K LOC
# typically produces 2K-10K nodes and 5K-30K edges. If you see very few edges,
# the extractors may not have found your code (check language support below).

# 4. Get context for a task
knowing context -task "refactor auth middleware" -format gcf

# 5. Check graph integrity
knowing fsck
```

### تحقّق من إعدادك

بعد الفهرسة، شغّل هذه الأوامر لتأكيد أن كل شيء يعمل:

```bash
# Show node/edge counts, repos, snapshots
knowing stats

# Search for a symbol you know exists in your code
knowing query "MyKnownFunction"

# Check graph integrity (should report 0 errors)
knowing fsck

# If results seem wrong, check if the graph is stale
knowing stale
```

إذا أظهر `knowing stats` صفر عُقد أو عددًا قليلًا جدًا من الحواف، فراجع
[استكشاف الأخطاء وإصلاحها](../../docs/guide/cli.md#troubleshooting) أدناه.

### مزيد من أوامر CLI

```bash
# Find affected tests
knowing test-scope -files internal/auth/middleware.go

# Explain why a symbol ranked where it did
knowing why -task "refactor auth" -symbol "SessionHandler"

# Prove a relationship exists (cryptographic Merkle proof)
knowing prove -source "AuthService" -target "SessionStore"

# Verify offline (no database needed)
knowing verify proof.json

# Check if the graph is stale (CI gate: exits 1 if stale)
knowing stale

# Supply chain audit (scan all files for suspicious patterns)
knowing audit-supply-chain --scan-all

# Remove a repo (evicts all data: nodes, edges, snapshots, feedback)
knowing remove ./path/to/repo
```

للاطلاع على المرجع الكامل للأوامر، راجع [مرجع CLI](../../docs/guide/cli.md).

### تكامل MCP

أضف خادم MCP إلى وكيلك. الإعداد واحد في كل مكان؛ يختلف مسار الملف فقط.

| الوكيل | ملف الإعداد |
|-------|-------------|
| **Claude Code** | `.mcp.json` (جذر المشروع) أو `~/.claude/mcp.json` (عام) |
| **Cursor** | `.cursor/mcp.json` |
| **Windsurf** | `~/.codeium/windsurf/mcp_config.json` |
| **VS Code** (Copilot، Continue، Cline، Roo) | `.vscode/mcp.json` |
| **Zed** | `~/.config/zed/settings.json` تحت `"context_servers"` |
| **Codex** (OpenAI) | `codex.json` أو الراية `--mcp-config` |
| **JetBrains** | Settings > Tools > MCP Servers |

```json
{
  "mcpServers": {
    "knowing": {
      "command": "knowing",
      "args": ["mcp", "--watch"],
      "transport": "stdio"
    }
  }
}
```

تعيد الراية `--watch` الفهرسة عند تغيّر الملفات. يستعلم وكيلك دائمًا عن بيانات حديثة. لا حاجة إلى `knowing index` يدوي أو مسار قاعدة بيانات: يفهرس خادم MCP مستودع git تلقائيًا عند أول تشغيل ويسجّله في السجل (roster) للجلسات المستقبلية.

التضمينات (Embeddings) مُعطّلة افتراضيًا (تأكّد حيادها في قياسات البداية الباردة). استخدم `--embeddings` لتمكينها عند التجربة. تحمل بنية الرسم البياني وفئات التكافؤ جودة الاسترجاع.

**ما الذي يحصل عليه وكيلك:** الأداة الأساسية هي `context_for_task`. عندما يستدعيها وكيلك بوصف مهمة، يُرجع knowing رموز شيفرة مرتّبة وذات صلة مُحزَّمة في ميزانية رموز. وهذا يحل محل حلقات grep-read. أدوات مفيدة أخرى: `blast_radius` (ما الذي يتعطّل إن غيّرت هذا؟)، و`test_scope` (أي اختبارات أُشغّل؟)، و`explain_symbol` (لماذا احتلّ هذا هذه المرتبة؟). راجع [مرجع أدوات MCP](../../docs/guide/mcp-tools.md) لجميع الأدوات الـ 28.

**تحقّق من أنه يعمل:**

1. ابدأ جلسة مع وكيلك
2. اسأل: *"استخدم أداة context_for_task للعثور على الرموز المرتبطة بـ [شيء محدد في شيفرتك]"*
3. ينبغي أن ترى رموزًا مرتّبة مع درجات ومسارات ملفات من قاعدة شيفرتك

إذا كانت النتائج فارغة: قد يكون المستودع لا يزال قيد الفهرسة (10-30 ثانية عند أول تشغيل). إذا بدت النتائج غير ذات صلة: استخدم أسماء رموز محددة في وصف مهمتك (مثلًا، "find the `AuthMiddleware` handler" لا "find auth code"). ويمكنك أيضًا التحقق من CLI:

```bash
knowing stats          # should show nodes and edges
knowing query "MyFunc" # should find symbols you recognize
```

لنقل HTTP (متعدد الوكلاء، وضع الخدمة الخفية daemon):

```bash
knowing serve -addr :8100 .
```

```json
{
  "mcpServers": {
    "knowing": {
      "url": "http://localhost:8100",
      "transport": "streamable-http"
    }
  }
}
```

---

## لماذا يعمل هذا

**Git يُصدّر الملفات (يعمل على إصداراتها). knowing يُصدّر فهم الشيفرة.**

النظام بأكمله مبني على فكرة واحدة: الهوية المعنونة بالمحتوى. كل رمز وعلاقة ولقطة مُجزّأة بـ SHA-256. هذا الاختيار الوحيد يمنحك:

- **كشف القِدَم مجانًا.** ملف مُتغيّر = تجزئة جديدة = الحواف القديمة معروفة دون مسح.
- **تخزين مؤقت مجانًا.** الجذر نفسه للحزمة = النتائج نفسها. تسريع 93x على الاستعلامات غير المُتغيّرة.
- **سلامة مجانًا.** تحقّق من جميع التجزئات المخزّنة واستمرارية سلسلة اللقطات. 98 مللي ثانية.
- **تاريخ مجانًا.** كل لقطة هي جذر Merkle مرتبط بـ git commit. سِر على السلسلة.
- **انتهاء صلاحية التغذية الراجعة مجانًا.** تخزّن التغذية الراجعة جذر Merkle للحزمة. تغيّر الشيفرة = تغيّر الجذر = التغذية الراجعة القديمة غير مرئية.
- **إثباتات مجانًا.** مسار Merkle من الورقة إلى الجذر هو إثبات تشفيري مكتفٍ بذاته.

| | Git | knowing |
|---|---|---|
| ماذا يُصدّر | محتويات الملفات | علاقات الشيفرة ومعناها |
| وحدة التخزين | blob | node + edge + provenance + confidence |
| الهوية | `sha256(content)` | `sha256("node\0" + repo + package + name + kind)` |
| اللقطة | شجرة من blobs | Merkle هرمي: repo -> package -> edge-type -> leaf |
| الفرق (Diff) | أي الأسطر تغيّرت | أي الحزم تغيّرت، وما الذي تعطّل، وما الجديد |
| التاريخ | كيف كانت تبدو الشيفرة | ما الذي فهمته قاعدة الشيفرة عن نفسها |

---

## كيف يعمل

```
+------------------------------------------------------------------+
|                         knowing daemon                            |
+----------------+------------------------+--------------------------+
|   Indexer      |     Graph Store        |      MCP Server          |
|                |                        |                          |
| 23 extractors  | Content-addressed      | 28 tools + 8 resources   |
| tree-sitter    | SQLite + Merkle tree   | stdio / HTTP (1.8s index)|
| LSP + SCIP     | 38 edge types          | GCF / GCB / JSON         |
| OTel traces    | Subgraph cache (93x)   | PackRoot dedup (99%)     |
|                | Embedding vector cache | Embedding re-ranker      |
|                | Community detection    | Supply chain audit       |
+----------------+------------------------+--------------------------+
```

مستويان:
- **التنفيذ:** يفهرس المستودعات، ويستخرج الرموز والعلاقات، ويستوعب الآثار، ويخزّن اللقطات.
- **الذكاء:** يحسب نطاق التأثير، وحُزم السياق، ونطاق الاختبار، والتغذية الراجعة، والمجتمعات من الرسم البياني المخزّن.

الحدّ الفاصل مهم: ميزات الذكاء تقرأ الرسم البياني وتُنتج نتائج مشتقة. لا يمكنها إفساد حقائق الرسم البياني. الترتيب السيّئ يُنتج توصية سيّئة؛ لكنه لا يمكنه إبطال إثبات.

---

## القدرات

### اللغات والتنسيقات

| اللغة/التنسيق | المُستخرِج | كشف الأُطر/الأنماط |
|---|---|---|
| Go | tree-sitter + `go/packages` + SCIP | net/http, gin, echo, chi, gorilla/mux |
| TypeScript/JavaScript | tree-sitter | Express.js, Fastify, Hono, NestJS, Next.js |
| Python | tree-sitter | Flask, FastAPI, Django |
| Rust | tree-sitter | Actix, Axum, Rocket |
| Java | tree-sitter | Spring annotations |
| C# | tree-sitter | ASP.NET attributes |
| Protocol Buffers | tree-sitter | تصريحات service، message، enum، RPC |
| Terraform (HCL) | tree-sitter | تصريحات resource، data، module، variable |
| SQL | tree-sitter | الجداول، والعروض، والدوال، والإجراءات، وحواف FK |
| Kubernetes YAML | yaml.v3 | deployments، services، configmaps، حواف label-selector |
| CloudFormation/SAM | yaml.v3 | resources، إحالات !Ref/!GetAtt/!Sub المتقاطعة |
| Docker Compose | yaml.v3 | services، ports، networks، روابط depends_on |
| GitHub Actions | yaml.v3 | workflows، jobs، steps، إحالات action |
| Serverless Framework | yaml.v3 | functions، events، إحالات resource |
| CSS/SCSS | tree-sitter | المحدِّدات، والخصائص المخصّصة، وتبعيات var() |
| أنماط Event/MQ | multi-language | نشر/اشتراك Kafka، NATS، SQS، RabbitMQ |
| OpenAPI/JSON Schema | json/yaml | endpoints، models، حلّ $ref |
| Dockerfile | parser | صور FROM الأساسية، تبعيات COPY --from متعددة المراحل، منافذ EXPOSE |
| Makefile | parser | تبعيات target، توجيهات include، إحالات variable |
| Helm Charts | yaml.v3 | تبعيات chart، إحالات template، حقن values |
| GitLab CI | yaml.v3 | job needs، قوالب extends، ملفات include، artifacts |
| package.json (npm) | json | dependencies، devDependencies، peerDependencies، scripts |
| GraphQL | parser | تعريفات الأنواع، إحالات أنواع الحقول، تطبيقات interface |
| Ruby | tree-sitter | الأصناف، والوحدات، وتعريفات التوابع، وحواف require |
| ملفات .env | parser | تصريحات متغيرات البيئة، والإحالات عبر الملفات |

تُطلَق جميع المُستخرِجات لكل ملف عبر الإرسال المتعدد (multi-dispatch)؛ وتُدمج النتائج. يُنتج tree-sitter حوافًا بثقة 0.7 (`ast_inferred`)؛ و`go/packages` وSCIP بثقة 0.95-1.0 (`ast_resolved`، `scip_resolved`).

### أدوات MCP

| الأداة | الغرض |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | بناء الرسم البياني وفحصه |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | فهم التأثير والمسارات |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | مقارنة حالات الرسم البياني ومراجعة التغييرات |
| `runtime_traffic`, `dead_routes`, `trace_stats` | الاستعلام عن العلاقات المرصودة في وقت التشغيل |
| `context_for_task`, `context_for_files`, `context_for_pr`, `explain_symbol` | سياق مرتّب للوكلاء |
| `ownership`, `ownership_query`, `test_scope`, `communities`, `plan_turn`, `feedback` | توجيه العمل، والاستعلام عن مالكي/مؤلفي الشيفرة، واختيار الاختبارات، وتحسين الترتيب |
| `prove`, `prove_absent`, `fsck` | إثباتات تشفيرية، وإثباتات غياب، والتحقق من السلامة |
| `untrack_repo` | إزالة جميع بيانات المستودع (العُقد، والحواف، والملفات، واللقطات، والتغذية الراجعة، وذاكرة المهام، وملاحظات الرسم البياني) |

مطالبات MCP: `refactor_safely`، `review_pr`، `investigate_dead_code`.

### موارد MCP

8 موارد للقراءة فقط لتوجيه الوكيل دون استدعاء أداة:

| المورد | ما الذي يُرجعه |
|---|---|
| `knowing://report` | حجم الرسم البياني، وأهم الأنواع، وعدد النقاط الساخنة، وعمر اللقطة |
| `knowing://schema` | أنواع العُقد، وأنواع الحواف، ومستويات المنشأ، وتنسيق التجزئة |
| `knowing://stats` | الأعداد حسب المستودع والنوع ونوع الحافة |
| `knowing://repos` | جميع المستودعات المتتبَّعة مع الأعداد ووقت آخر فهرسة |
| `knowing://session` | استدعاءات السياق، والرموز المُقدَّمة، وإصابات/إخفاقات الذاكرة المؤقتة، ووقت التشغيل |
| `knowing://index-health` | حالة سليم/قديم/تالف، والتحقق من السلامة |
| `knowing://communities` | قائمة المجتمعات مع تماسكها وجذور Merkle |
| `knowing://community/{id}` | تفاصيل مجتمع مفرد (قالب مورد) |

---

## تنسيقات الشبكة (Wire Formats)

| التنسيق | الغرض | التوفير مقابل JSON |
|---|---|---|
| **GCF** (Graph Compact Format) | استهلاك LLM: موجّه للأسطر، حقول موضعية | رموز أقل بنسبة 84% |
| **GCB** (Graph Compact Binary) | نقل الخدمات والتخزين المؤقت: varint، مسبوق بالطول | بايتات أقل بنسبة 74% |
| **JSON** | تصحيح بشري، مستهلكون عامّون | خط الأساس |

يستخدم GCF حقولًا مفصولة بـ `|` ومعرّفات محلية (`$1 -> $3`) بدلاً من الأسماء المؤهّلة المتكررة. قابل للتحليل بواسطة نماذج LLM مع إدراج سياق رسم بياني أكثر بـ 5x ضمن ميزانية الرموز نفسها. ويقلّل إزالةُ التكرار ذاتُ الحالة على مستوى الجلسة الرموزَ المتكررة بنسبة 47%.

---

## الحدود الحالية

- يتتبّع نطاق التأثير الساكن حواف `calls`؛ وتوفّر أنواع الحواف الأخرى سياقًا، لا اجتيازًا.
- تتطلّب أدوات وقت التشغيل استيعاب آثار OpenTelemetry؛ وبدون آثار لا تملك رصدًا.
- إثراء LSP: Go، TypeScript، Python، Rust، Java، C#. يُكتشف تلقائيًا من علامات المشروع. وتتراجع اللغات الأخرى إلى tree-sitter.
- التضمينات مُعطّلة افتراضيًا (تأكّد حيادها في قياسات البداية الباردة، الجلسة 23). استخدم `--embeddings` للاشتراك.

---

## التوثيق

| الوثيقة | المحتويات |
|---|---|
| [مقدمة](../../docs/guide/introduction.md) | كيف يعمل، شرح خط أنابيب الاسترجاع، جولة في 5 دقائق |
| [البنية المعمارية](../../docs/architecture/) | تصميم النظام، والمخططات (schemas)، والعَنْوَنة بالمحتوى، ونموذج الخدمة الخفية |
| [الميزات](../../docs/guide/features.md) | جرد التنفيذ، ونقاط الدخول، والقيود |
| [التدقيق والامتثال](../../docs/guide/audit-compliance.md) | إثباتات Merkle، وfsck، وسلسلة اللقطات، وبوابات CI |
| [مرجع CLI](../../docs/guide/cli.md) | الأوامر، والرايات، والأمثلة، و[استكشاف الأخطاء وإصلاحها](../../docs/guide/cli.md#troubleshooting) |
| [أدوات MCP](../../docs/guide/mcp-tools.md) | مخططات الأدوات، والمعاملات، وتنسيقات الإرجاع |
| [أنواع الحواف](../../docs/architecture/edge-types.md) | دلالات العلاقات والمنشأ |
| [تحزيم السياق](../../docs/architecture/context-packing.md) | RWR، وHITS، والترتيب، وميزنة الرموز |
| [مُعيد ترتيب التضمينات](../../docs/architecture/embedding-reranker.md) | الاستدلال المحلي، وذاكرة المتّجهات المؤقتة، وملف الكمون |
| [آثار وقت التشغيل](../../docs/operations/runtime-traces.md) | استيعاب OTel وثقة وقت التشغيل |
| [تنسيقات الشبكة](../../docs/architecture/wire-formats.md) | تنسيقات GCF، وGCB، وJSON والقياسات المرجعية |
| [خارطة الطريق](../../docs/roadmap.md) | مسارات العمل المكتملة والأولويات التالية |
| [القياسات المرجعية](../../bench/README.md) | قياسات قيمة قابلة لإعادة الإنتاج مع عقود أداء |
| [البحث](../../docs/research/content-addressing-as-computation-primitive.md) | الأطروحة التي بُني عليها knowing: العَنْوَنة بالمحتوى كبدائية حوسبة ([DOI: 10.5281/zenodo.20342255](https://zenodo.org/records/20342255)) |
| [Hooks](../../hooks/README.md) | تكامل خطّافات Claude Code |

## الترخيص

Apache-2.0 (c) 2026 Dayna Blackwell / Blackwell Systems. راجع LICENSE وNOTICE.
