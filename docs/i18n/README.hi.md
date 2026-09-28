[English](../../README.md) · [简体中文](README.zh-CN.md) · [Русский](README.ru.md) · **हिन्दी** · [العربية](README.ar.md)

<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
  <a href="https://zenodo.org/records/20342255"><img src="https://zenodo.org/badge/DOI/10.5281/zenodo.20342255.svg" alt="DOI"></a>
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-28%20tools%20%2B%208%20resources-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/languages_and_formats-26-blue.svg" alt="Languages and Formats"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache_2.0-blue.svg" alt="License"></a>
</p>

---

स्वयं-अनुकूलित होने वाला कोड इंटेलिजेंस इंजन। अपने स्वयं के ग्राफ़ के घनत्व को देखता है और रिट्रीवल रणनीति को स्वचालित रूप से समायोजित करता है। 38 एज प्रकार, 28 MCP tools, 263 समतुल्यता वर्ग, क्रिप्टोग्राफ़िक प्रमाण। पैमाने के साथ अधिक बुद्धिमान होता है, न कि अधिक मूर्ख।

---

> [!NOTE]
> **प्रकाशित शोध पर आधारित:** [Content-Addressing as a Computation Primitive for Software Relationship Intelligence](https://zenodo.org/records/20342255) (DOI: 10.5281/zenodo.20342255)

आपका आर्किटेक्चर डायग्राम कहता है कि सेवा A सेवा B को कॉल करती है। क्या आप इसे सिद्ध कर सकते हैं?

**knowing कर सकता है।** यह निकाले गए कोड संबंधों का एक content-addressed ग्राफ़ बनाता है, इसे किसी git commit से बंधे Merkle tree के रूप में स्नैपशॉट करता है, और ऐसे क्रिप्टोग्राफ़िक प्रमाण उत्पन्न करता है जो ऑफ़लाइन सत्यापित होते हैं। Agent इसका उपयोग रैंक किए गए संदर्भ के लिए करते हैं। सुरक्षा टीमें इसका उपयोग ऑडिट के लिए करती हैं। प्लेटफ़ॉर्म टीमें इसका उपयोग कोड की तुलना production traces से करने के लिए करती हैं।

आप जितनी बार इसका उपयोग करते हैं, यह उतना ही बेहतर होता जाता है। जब कोड बदलता है, तो पुराना ज्ञान स्वचालित रूप से समाप्त हो जाता है।

```bash
brew install blackwell-systems/tap/knowing
```

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

बस इतना ही। MCP server पहली बार लॉन्च होने पर आपके repo को स्वचालित रूप से इंडेक्स करता है। न मॉडल डाउनलोड, न API keys। आपके agent के पास अब रैंक किया गया संदर्भ, blast radius, टेस्ट स्कोप, और अंतर्निहित नॉइज़ डिमोशन है जो सक्रिय सत्रों के दौरान परिणामों को बेहतर बनाता है।

**सत्यापित करें कि यह काम करता है:** अपने agent से पूछें: *"context_for_task tool का उपयोग करके [कोई ऐसी चीज़ जो आप जानते हैं कि आपके कोड में मौजूद है] से संबंधित symbols खोजें।"* आपको अपने कोडबेस से स्कोर और फ़ाइल पथ के साथ रैंक किए गए symbols दिखने चाहिए। यदि परिणाम खाली हैं, तो repo अभी भी इंडेक्स हो रहा है (पहली बार लॉन्च पर 10-30 सेकंड)। यदि परिणाम असंबंधित लगते हैं, तो [समस्या निवारण](docs/guide/cli.md#troubleshooting) देखें।

> **AI agent का उपयोग नहीं कर रहे?** नीचे [CLI उपयोग](#path-b-cli-usage-explore-the-graph-yourself) पर जाएँ।

| आप चाहते हैं... | यहाँ से शुरू करें |
|---|---|
| अपने AI agent को ग्राफ़-रैंक किया गया संदर्भ दें | [MCP सेटअप](#mcp-integration) |
| CLI से ग्राफ़ का अन्वेषण करें | [CLI उपयोग](#path-b-cli-usage-explore-the-graph-yourself) |
| समझें कि रिट्रीवल कैसे काम करता है | [परिचय](docs/guide/introduction.md) |
| क्रिप्टोग्राफ़िक प्रमाणों के साथ ऑडिट करें | [ऑडिट और अनुपालन](docs/guide/audit-compliance.md) |

---

## तीन चीज़ें, एक आर्किटेक्चर

knowing एक ही आधार (श्रेणीबद्ध Merkle trees के साथ content-addressed ग्राफ़) पर बने तीन उत्पाद हैं:

**1. AI agents के लिए संदर्भ इंजन**
एक कॉल किसी कार्य के लिए सबसे प्रासंगिक symbols लौटाती है, जो ग्राफ़ केंद्रीयता, नवीनता, और सीखी गई उपयोगिता के आधार पर रैंक किए जाते हैं, और आपके token बजट में फ़िट होने के लिए पैक किए जाते हैं। 263 फ़्रेमवर्क समतुल्यता वर्ग तब शब्दावली की खाई को पाटते हैं जब keywords विफल हो जाते हैं। 47% कम tool कॉल। 84% कम tokens। परिणाम फ़ीडबैक के साथ बेहतर होते हैं।

**2. अनुपालन के लिए ऑडिट प्रिमिटिव**
प्रत्येक ग्राफ़ स्थिति किसी git commit से बंधा एक Merkle root है। `knowing prove` एक क्रिप्टोग्राफ़िक प्रमाण उत्पन्न करता है कि कोई संबंध मौजूद था। `knowing verify` इसे ऑफ़लाइन जाँचता है। `knowing fsck` पूरे ग्राफ़ को 98ms में सत्यापित करता है। सप्लाई चेन डिटेक्शन क्रेडेंशियल एक्सेस, प्रोसेस स्पॉनिंग, और नेटवर्क एक्सफ़िल्ट्रेशन एजेस निकालता है ताकि संरचनात्मक रूप से संदिग्ध कोड को चिह्नित किया जा सके।

**3. नॉइज़ डिमोशन जो सीखता है**
जो symbols लौटाए गए लेकिन agent द्वारा कभी उपयोग नहीं किए गए, उन्हें भविष्य की क्वेरीज़ में डिमोट कर दिया जाता है। जब कोड बदलता है, तो फ़ीडबैक स्वचालित रूप से समाप्त हो जाता है (package Merkle roots के माध्यम से सत्यापित)। सिस्टम सक्रिय सत्रों के दौरान अधिक सटीक होता जाता है। यही वह गुण है जिसके इर्द-गिर्द knowing बनाया गया है।

**ये अलग-अलग फ़ीचर नहीं हैं।** ये content-addressing के संरचनात्मक परिणाम हैं: वही hash जो संदर्भ को cacheable बनाता है, उसे provable भी बनाता है, और वही Merkle root जो पुरानापन पहचानता है, पुराने फ़ीडबैक को भी समाप्त करता है।

---

## यह किन प्रश्नों का उत्तर देता है

**आपके agent के लिए:**
- "मैं यह फ़ंक्शन बदल रहा हूँ। क्या टूटेगा?" (callers, tests, routes, repos में blast radius)
- "इस कार्य के लिए मुझे 50,000 tokens का संदर्भ दें।" (ग्राफ़-रैंक किया गया, grep-खोजा गया नहीं)
- "कौन से टेस्ट चलने चाहिए?" (कॉल-ग्राफ़ ट्रैवर्सल, 98% परिशुद्धता)

**आपकी प्लेटफ़ॉर्म टीम के लिए:**
- "क्या यह route production में उपयोग होता है?" (स्थैतिक विश्लेषण + OTel रनटाइम traces)
- "किसी विशिष्ट स्नैपशॉट पर सेवा ग्राफ़ कैसा दिखता था?" (स्नैपशॉट श्रृंखला, प्रत्येक root किसी git commit से बंधा)

**आपकी सुरक्षा टीम के लिए:**
- "सिद्ध करें कि इस commit पर सेवा A सेवा B को कॉल करती है।" (Merkle प्रमाण, ऑफ़लाइन सत्यापनीय)
- "सिद्ध करें कि यह निर्भरता मौजूद नहीं है।" (सॉर्ट किए गए leaves के माध्यम से अनुपस्थिति प्रमाण)
- "एक अनुपालन रिपोर्ट उत्पन्न करें।" (`knowing audit -proofs`, एक कमांड)
- "क्या यह package क्रेडेंशियल पढ़ता है और प्रोसेस स्पॉन करता है?" (`knowing audit-supply-chain --scan-all`)

---

## आँकड़े

| क्या | परिणाम |
|---|---:|
| क्रॉस-सिस्टम रिट्रीवल | **P@10=0.330 कोल्ड स्टार्ट** (302 कार्य, 17 repos, 8 भाषाएँ) |
| प्रतिस्पर्धियों की तुलना में | 3.79x codegraph (19K stars), 6.00x GitNexus, 6.35x Gortex, 22.0x grep |
| समतुल्यता वर्ग | 277 हाथ से तैयार किए गए + उपयोग से सीखे गए, शब्दावली को symbols से जोड़ते हुए (+57% P@10) |
| नॉइज़ डिमोशन | प्रति-क्लस्टर अंतर्निहित फ़ीडबैक: R@10 +5.2%, MRR +12.6% (Django 5 राउंड) |
| बचाए गए tool कॉल | 47% कम (एक संदर्भ कॉल बार-बार के grep+read को प्रतिस्थापित करती है) |
| token बचत | 84% कम tokens (GCF wire format) |
| दोहराई गई क्वेरी की गति | 93x तेज़ (Merkle-keyed subgraph cache) |
| Merkle diff | 100K एजेस पर पूर्ण एज स्कैन से 517x तेज़ |
| टेस्ट स्कोप | 98% परिशुद्धता, 82% रिकॉल |
| ग्राफ़ अखंडता जाँच | 98ms (24,936 एजेस) |
| प्रमाण उत्पादन | 72us उत्पन्न, 1.2us सत्यापित |
| फ़ीडबैक समाप्ति | कोड परिवर्तन पर 100% समाप्त, 11% ओवरहेड |
| इंडेक्सिंग थ्रूपुट | 16 repos (8 भाषाएँ) ~60s में |
| भाषा कवरेज | 16/16 repos पास (Go, Python, TS, Rust, Java, C#, Ruby, multi) |
| एज प्रकार | 38 (सप्लाई चेन सहित: reads_env, executes_process) |

सभी benchmarks पुनरुत्पादनीय हैं। क्रॉस-सिस्टम benchmark (P@10=0.330) सटीक commits पर पिन किए गए 17 repos का उपयोग करता है, जिसमें पूर्ण रूप से नए सिरे से पुनरुत्पादन के लिए एक [corpus manifest](bench/cross-system/corpus/MANIFEST.yaml) और [setup script](bench/cross-system/corpus/corpus-setup.sh) शामिल है। प्रोटोकॉल विवरण के लिए [METHODOLOGY.md](bench/cross-system/METHODOLOGY.md) देखें।

---

## त्वरित शुरुआत

### पथ A: MCP server (AI agents के लिए अनुशंसित)

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

### पथ B: CLI उपयोग (ग्राफ़ का स्वयं अन्वेषण करें)

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

### अपना सेटअप सत्यापित करें

इंडेक्सिंग के बाद, यह पुष्टि करने के लिए ये कमांड चलाएँ कि सब कुछ काम कर रहा है:

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

यदि `knowing stats` शून्य nodes या बहुत कम एजेस दिखाता है, तो नीचे
[समस्या निवारण](docs/guide/cli.md#troubleshooting) देखें।

### अधिक CLI कमांड

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

पूर्ण कमांड संदर्भ के लिए, [CLI संदर्भ](docs/guide/cli.md) देखें।

### MCP एकीकरण

MCP server को अपने agent में जोड़ें। कॉन्फ़िग हर जगह एक जैसा है; केवल फ़ाइल पथ भिन्न है।

| Agent | कॉन्फ़िग फ़ाइल |
|-------|-------------|
| **Claude Code** | `.mcp.json` (प्रोजेक्ट रूट) या `~/.claude/mcp.json` (ग्लोबल) |
| **Cursor** | `.cursor/mcp.json` |
| **Windsurf** | `~/.codeium/windsurf/mcp_config.json` |
| **VS Code** (Copilot, Continue, Cline, Roo) | `.vscode/mcp.json` |
| **Zed** | `~/.config/zed/settings.json` के अंतर्गत `"context_servers"` |
| **Codex** (OpenAI) | `codex.json` या `--mcp-config` फ़्लैग |
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

`--watch` फ़्लैग फ़ाइल परिवर्तनों पर पुनः इंडेक्स करता है। आपका agent हमेशा ताज़ा डेटा क्वेरी करता है। किसी मैनुअल `knowing index` या डेटाबेस पथ की आवश्यकता नहीं: MCP server पहली बार लॉन्च होने पर git repository को स्वचालित रूप से इंडेक्स करता है और भविष्य के सत्रों के लिए इसे roster में पंजीकृत करता है।

Embeddings डिफ़ॉल्ट रूप से बंद हैं (कोल्ड-स्टार्ट benchmarks पर तटस्थ के रूप में पुष्ट)। प्रयोग करने के लिए सक्षम करने हेतु `--embeddings` का उपयोग करें। ग्राफ़ संरचना और समतुल्यता वर्ग रिट्रीवल गुणवत्ता को वहन करते हैं।

**आपके agent को क्या मिलता है:** मुख्य tool `context_for_task` है। जब आपका agent किसी कार्य विवरण के साथ इसे कॉल करता है, तो knowing token बजट में पैक किए गए रैंक किए गए, प्रासंगिक कोड symbols लौटाता है। यह grep-read लूप्स को प्रतिस्थापित करता है। अन्य उपयोगी tools: `blast_radius` (यदि मैं इसे बदलूँ तो क्या टूटेगा?), `test_scope` (कौन से टेस्ट चलाने हैं?), `explain_symbol` (यह यहाँ क्यों रैंक हुआ?)। सभी 28 tools के लिए [MCP Tools संदर्भ](docs/guide/mcp-tools.md) देखें।

**सत्यापित करें कि यह काम करता है:**

1. अपने agent के साथ एक सत्र शुरू करें
2. पूछें: *"context_for_task tool का उपयोग करके [आपके कोड में कोई विशिष्ट चीज़] से संबंधित symbols खोजें"*
3. आपको अपने कोडबेस से स्कोर और फ़ाइल पथ के साथ रैंक किए गए symbols दिखने चाहिए

यदि परिणाम खाली हैं: repo अभी भी इंडेक्स हो रहा हो सकता है (पहली बार लॉन्च पर 10-30 सेकंड)। यदि परिणाम असंबंधित लगते हैं: अपने कार्य विवरण में विशिष्ट symbol नामों का उपयोग करें (उदाहरण के लिए, "find the `AuthMiddleware` handler" न कि "find auth code")। आप CLI से भी सत्यापित कर सकते हैं:

```bash
knowing stats          # should show nodes and edges
knowing query "MyFunc" # should find symbols you recognize
```

HTTP transport (मल्टी-agent, daemon मोड) के लिए:

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

## यह क्यों काम करता है

**Git फ़ाइलों का संस्करण बनाता है। knowing कोड की समझ का संस्करण बनाता है।**

पूरा सिस्टम एक विचार पर बना है: content-addressed पहचान। प्रत्येक symbol, संबंध, और स्नैपशॉट SHA-256 hashed है। यह एक विकल्प आपको देता है:

- **मुफ़्त में पुरानापन पहचान।** बदली गई फ़ाइल = नया hash = बिना स्कैन किए पुराने एजेस ज्ञात होते हैं।
- **मुफ़्त में कैशिंग।** वही package root = वही परिणाम। अपरिवर्तित क्वेरीज़ पर 93x गति वृद्धि।
- **मुफ़्त में अखंडता।** सभी संग्रहीत hashes और स्नैपशॉट श्रृंखला निरंतरता सत्यापित करें। 98ms।
- **मुफ़्त में इतिहास।** प्रत्येक स्नैपशॉट किसी git commit से बंधा एक Merkle root है। श्रृंखला पर चलें।
- **मुफ़्त में फ़ीडबैक समाप्ति।** फ़ीडबैक package Merkle root संग्रहीत करता है। कोड परिवर्तन = root परिवर्तन = पुराना फ़ीडबैक अदृश्य।
- **मुफ़्त में प्रमाण।** leaf से root तक का Merkle पथ अपने आप में एक स्वयं-निहित क्रिप्टोग्राफ़िक प्रमाण है।

| | Git | knowing |
|---|---|---|
| यह किसका संस्करण बनाता है | फ़ाइल सामग्री | कोड संबंध और उनका अर्थ |
| भंडारण की इकाई | blob | node + edge + provenance + confidence |
| पहचान | `sha256(content)` | `sha256("node\0" + repo + package + name + kind)` |
| स्नैपशॉट | blobs का tree | श्रेणीबद्ध Merkle: repo -> package -> edge-type -> leaf |
| Diff | कौन सी लाइनें बदलीं | कौन से packages बदले, क्या टूटा, क्या नया है |
| इतिहास | कोड कैसा दिखता था | कोडबेस स्वयं के बारे में क्या समझता था |

---

## यह कैसे काम करता है

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

दो तल:
- **निष्पादन:** repos को इंडेक्स करता है, symbols और संबंध निकालता है, traces को इंजेस्ट करता है, स्नैपशॉट संग्रहीत करता है।
- **इंटेलिजेंस:** संग्रहीत ग्राफ़ से blast radius, संदर्भ packs, टेस्ट स्कोप, फ़ीडबैक, communities की गणना करता है।

यह सीमा मायने रखती है: इंटेलिजेंस फ़ीचर ग्राफ़ को पढ़ते हैं और व्युत्पन्न परिणाम उत्पन्न करते हैं। वे ग्राफ़ के तथ्यों को दूषित नहीं कर सकते। एक खराब रैंकिंग एक खराब सिफ़ारिश उत्पन्न करती है; यह किसी प्रमाण को अमान्य नहीं कर सकती।

---

## क्षमताएँ

### भाषाएँ और प्रारूप

| भाषा/प्रारूप | Extractor | फ़्रेमवर्क/पैटर्न डिटेक्शन |
|---|---|---|
| Go | tree-sitter + `go/packages` + SCIP | net/http, gin, echo, chi, gorilla/mux |
| TypeScript/JavaScript | tree-sitter | Express.js, Fastify, Hono, NestJS, Next.js |
| Python | tree-sitter | Flask, FastAPI, Django |
| Rust | tree-sitter | Actix, Axum, Rocket |
| Java | tree-sitter | Spring annotations |
| C# | tree-sitter | ASP.NET attributes |
| Protocol Buffers | tree-sitter | service, message, enum, RPC घोषणाएँ |
| Terraform (HCL) | tree-sitter | resource, data, module, variable घोषणाएँ |
| SQL | tree-sitter | tables, views, functions, procedures, FK एजेस |
| Kubernetes YAML | yaml.v3 | deployments, services, configmaps, label-selector एजेस |
| CloudFormation/SAM | yaml.v3 | resources, !Ref/!GetAtt/!Sub क्रॉस-रेफ़रेंस |
| Docker Compose | yaml.v3 | services, ports, networks, depends_on लिंक |
| GitHub Actions | yaml.v3 | workflows, jobs, steps, action रेफ़रेंस |
| Serverless Framework | yaml.v3 | functions, events, resource रेफ़रेंस |
| CSS/SCSS | tree-sitter | selectors, custom properties, var() निर्भरताएँ |
| Event/MQ पैटर्न | multi-language | Kafka, NATS, SQS, RabbitMQ publish/subscribe |
| OpenAPI/JSON Schema | json/yaml | endpoints, models, $ref रिज़ॉल्यूशन |
| Dockerfile | parser | FROM base images, COPY --from multi-stage निर्भरताएँ, EXPOSE ports |
| Makefile | parser | target निर्भरताएँ, include directives, variable रेफ़रेंस |
| Helm Charts | yaml.v3 | chart निर्भरताएँ, template रेफ़रेंस, values injection |
| GitLab CI | yaml.v3 | job needs, extends templates, include files, artifacts |
| package.json (npm) | json | dependencies, devDependencies, peerDependencies, scripts |
| GraphQL | parser | type definitions, field type रेफ़रेंस, interface implementations |
| Ruby | tree-sitter | classes, modules, method definitions, require एजेस |
| .env फ़ाइलें | parser | environment variable घोषणाएँ, क्रॉस-फ़ाइल रेफ़रेंस |

सभी extractors मल्टी-डिस्पैच के माध्यम से प्रति फ़ाइल फ़ायर होते हैं; परिणाम मर्ज किए जाते हैं। Tree-sitter 0.7 confidence (`ast_inferred`) पर एजेस उत्पन्न करता है; `go/packages` और SCIP 0.95-1.0 (`ast_resolved`, `scip_resolved`) पर।

### MCP Tools

| Tool | उद्देश्य |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | ग्राफ़ बनाना और निरीक्षण करना |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | प्रभाव और पथ समझना |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | ग्राफ़ स्थितियों की तुलना करना और परिवर्तनों की समीक्षा करना |
| `runtime_traffic`, `dead_routes`, `trace_stats` | रनटाइम-प्रेक्षित संबंधों की क्वेरी करना |
| `context_for_task`, `context_for_files`, `context_for_pr`, `explain_symbol` | agents के लिए रैंक किया गया संदर्भ |
| `ownership`, `ownership_query`, `test_scope`, `communities`, `plan_turn`, `feedback` | कार्य रूट करना, कोड मालिकों/लेखकों की क्वेरी करना, टेस्ट चुनना, रैंकिंग सुधारना |
| `prove`, `prove_absent`, `fsck` | क्रिप्टोग्राफ़िक प्रमाण, अनुपस्थिति प्रमाण, अखंडता सत्यापन |
| `untrack_repo` | किसी repository के सभी डेटा को हटाना (nodes, edges, files, snapshots, feedback, task memory, graph notes) |

MCP prompts: `refactor_safely`, `review_pr`, `investigate_dead_code`।

### MCP Resources

बिना tool कॉल के agent अभिविन्यास हेतु 8 केवल-पढ़ने योग्य संसाधन:

| संसाधन | यह क्या लौटाता है |
|---|---|
| `knowing://report` | ग्राफ़ आकार, शीर्ष प्रकार, hotspot गणना, स्नैपशॉट आयु |
| `knowing://schema` | Node प्रकार, edge प्रकार, provenance स्तर, hash प्रारूप |
| `knowing://stats` | repo, प्रकार, और edge प्रकार के अनुसार गणनाएँ |
| `knowing://repos` | सभी ट्रैक किए गए repos गणनाओं और अंतिम-इंडेक्स समय के साथ |
| `knowing://session` | संदर्भ कॉल, परोसे गए symbols, cache hits/misses, uptime |
| `knowing://index-health` | Healthy/stale/corrupted स्थिति, अखंडता जाँच |
| `knowing://communities` | cohesion और Merkle roots के साथ community सूची |
| `knowing://community/{id}` | एकल community विवरण (resource template) |

---

## Wire Formats

| प्रारूप | उद्देश्य | JSON की तुलना में बचत |
|---|---|---|
| **GCF** (Graph Compact Format) | LLM उपभोग: लाइन-उन्मुख, स्थितीय फ़ील्ड | 84% कम tokens |
| **GCB** (Graph Compact Binary) | सेवा transport और caching: varint, length-prefixed | 74% कम bytes |
| **JSON** | मानव डिबगिंग, सामान्य उपभोक्ता | आधार रेखा |

GCF दोहराए गए योग्य नामों के बजाय `|`-पृथक्कृत फ़ील्ड और स्थानीय IDs (`$1 -> $3`) का उपयोग करता है। LLMs द्वारा पार्स करने योग्य, साथ ही उसी token बजट में 5x अधिक ग्राफ़ संदर्भ फ़िट करता है। सत्र-अवस्थित deduplication दोहराए गए symbols को 47% कम करता है।

---

## वर्तमान सीमाएँ

- स्थैतिक blast radius `calls` एजेस का अनुसरण करता है; अन्य edge प्रकार संदर्भ प्रदान करते हैं, ट्रैवर्सल नहीं।
- रनटाइम tools को OpenTelemetry trace इंजेशन की आवश्यकता होती है; traces के बिना उनके पास कोई प्रेक्षण नहीं है।
- LSP संवर्धन: Go, TypeScript, Python, Rust, Java, C#। प्रोजेक्ट markers से स्वचालित रूप से पहचाना जाता है। अन्य tree-sitter पर वापस आते हैं।
- Embeddings डिफ़ॉल्ट रूप से बंद हैं (कोल्ड-स्टार्ट benchmarks पर तटस्थ के रूप में पुष्ट, session 23)। ऑप्ट इन करने के लिए `--embeddings` का उपयोग करें।

---

## दस्तावेज़ीकरण

| दस्तावेज़ | विषय-वस्तु |
|---|---|
| [परिचय](docs/guide/introduction.md) | यह कैसे काम करता है, रिट्रीवल पाइपलाइन समझाई गई, 5-मिनट का वॉकथ्रू |
| [आर्किटेक्चर](docs/architecture/) | सिस्टम डिज़ाइन, schemas, content addressing, daemon मॉडल |
| [फ़ीचर](docs/guide/features.md) | कार्यान्वयन सूची, entry points, सीमाएँ |
| [ऑडिट और अनुपालन](docs/guide/audit-compliance.md) | Merkle प्रमाण, fsck, स्नैपशॉट श्रृंखला, CI gates |
| [CLI संदर्भ](docs/guide/cli.md) | कमांड, फ़्लैग, उदाहरण, [समस्या निवारण](docs/guide/cli.md#troubleshooting) |
| [MCP Tools](docs/guide/mcp-tools.md) | Tool schemas, पैरामीटर, रिटर्न प्रारूप |
| [Edge Types](docs/architecture/edge-types.md) | संबंध शब्दार्थ और provenance |
| [Context Packing](docs/architecture/context-packing.md) | RWR, HITS, रैंकिंग, token बजटिंग |
| [Embedding Re-ranker](docs/architecture/embedding-reranker.md) | स्थानीय inference, vector cache, latency प्रोफ़ाइल |
| [Runtime Traces](docs/operations/runtime-traces.md) | OTel इंजेशन और रनटाइम confidence |
| [Wire Formats](docs/architecture/wire-formats.md) | GCF, GCB, JSON प्रारूप और benchmarks |
| [Roadmap](docs/roadmap.md) | पूर्ण किए गए workstreams और अगली प्राथमिकताएँ |
| [Benchmarks](bench/README.md) | प्रदर्शन contracts के साथ पुनरुत्पादनीय मूल्य benchmarks |
| [Research](docs/research/content-addressing-as-computation-primitive.md) | वह थीसिस जिस पर knowing बना है: content-addressing एक computation primitive के रूप में ([DOI: 10.5281/zenodo.20342255](https://zenodo.org/records/20342255)) |
| [Hooks](hooks/README.md) | Claude Code hook एकीकरण |

## लाइसेंस

Apache-2.0 (c) 2026 Dayna Blackwell / Blackwell Systems। LICENSE और NOTICE देखें।
