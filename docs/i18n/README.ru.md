[English](../../README.md) · [简体中文](README.zh-CN.md) · **Русский** · [हिन्दी](README.hi.md)

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

Самонастраивающийся движок анализа кода. Наблюдает за плотностью собственного графа и автоматически подстраивает стратегию поиска. 38 типов рёбер, 28 инструментов MCP, 263 класса эквивалентности, криптографические доказательства. С ростом масштаба становится умнее, а не глупее.

---

> [!NOTE]
> **Основан на опубликованном исследовании:** [Content-Addressing as a Computation Primitive for Software Relationship Intelligence](https://zenodo.org/records/20342255) (DOI: 10.5281/zenodo.20342255)

Ваша архитектурная диаграмма утверждает, что сервис A вызывает сервис B. Можете это доказать?

**knowing может.** Он строит контентно-адресуемый граф извлечённых связей кода, снимает его снимок в виде дерева Меркла, привязанного к git-коммиту, и генерирует криптографические доказательства, проверяемые офлайн. Агенты используют его для ранжированного контекста. Команды безопасности используют его для аудита. Платформенные команды используют его, чтобы сопоставлять код с продакшн-трейсами.

Он становится лучше с каждым использованием. Когда код меняется, устаревшие знания истекают автоматически.

```bash
brew install blackwell-systems/tap/knowing
```

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

Вот и всё. Сервер MCP автоматически индексирует ваш репозиторий при первом запуске. Ни загрузки моделей, ни API-ключей. Ваш агент теперь получает ранжированный контекст, радиус поражения (blast radius), охват тестов и неявное подавление шума, которое улучшает результаты в ходе активных сессий.

**Проверьте, что это работает:** Попросите вашего агента: *«Используй инструмент context_for_task, чтобы найти символы, связанные с [чем-то, что точно есть в вашем коде].»* Вы должны увидеть ранжированные символы с оценками и путями к файлам из вашей кодовой базы. Если результаты пусты, репозиторий ещё индексируется (10-30 секунд при первом запуске). Если результаты кажутся нерелевантными, см. [Устранение неполадок](docs/guide/cli.md#troubleshooting).

> **Не используете AI-агента?** Переходите к [Использованию CLI](#path-b-cli-usage-explore-the-graph-yourself) ниже.

| Вы хотите... | Начните здесь |
|---|---|
| Дать вашему AI-агенту контекст, ранжированный по графу | [Настройка MCP](#mcp-integration) |
| Исследовать граф из CLI | [Использование CLI](#path-b-cli-usage-explore-the-graph-yourself) |
| Понять, как работает поиск | [Введение](docs/guide/introduction.md) |
| Проводить аудит с криптографическими доказательствами | [Аудит и соответствие](docs/guide/audit-compliance.md) |

---

## Три вещи, одна архитектура

knowing — это три продукта, построенные на одной основе (контентно-адресуемый граф с иерархическими деревьями Меркла):

**1. Движок контекста для AI-агентов**
Один вызов возвращает наиболее релевантные для задачи символы, ранжированные по центральности в графе, недавности и усвоенной полезности, упакованные под ваш бюджет токенов. 263 класса эквивалентности фреймворков устраняют разрыв в лексике, когда ключевые слова не срабатывают. На 47% меньше вызовов инструментов. На 84% меньше токенов. Результаты улучшаются с обратной связью.

**2. Примитив аудита для соответствия требованиям**
Каждое состояние графа — это корень Меркла, привязанный к git-коммиту. `knowing prove` генерирует криптографическое доказательство того, что связь существовала. `knowing verify` проверяет его офлайн. `knowing fsck` проверяет весь граф за 98 мс. Обнаружение цепочки поставок извлекает рёбра доступа к учётным данным, порождения процессов и сетевой эксфильтрации, чтобы помечать структурно подозрительный код.

**3. Подавление шума, которое учится**
Символы, которые были возвращены, но так и не использованы агентом, понижаются в приоритете при будущих запросах. Когда код меняется, обратная связь истекает автоматически (проверяется через корни Меркла пакетов). Система становится точнее в ходе активных сессий. Именно это свойство лежит в основе knowing.

**Это не отдельные функции.** Это структурные следствия контентной адресации: тот же хеш, который делает контекст кешируемым, делает его и доказуемым, а тот же корень Меркла, который обнаруживает устаревание, заставляет истекать и устаревшую обратную связь.

---

## На какие вопросы он отвечает

**Для вашего агента:**
- «Я меняю эту функцию. Что сломается?» (радиус поражения по вызывающим сторонам, тестам, маршрутам, репозиториям)
- «Дай мне 50 000 токенов контекста для этой задачи.» (ранжированного по графу, а не найденного через grep)
- «Какие тесты нужно запустить?» (обход графа вызовов, точность 98%)

**Для вашей платформенной команды:**
- «Используется ли этот маршрут в продакшене?» (статический анализ + рантайм-трейсы OTel)
- «Как выглядел граф сервисов в определённый момент снимка?» (цепочка снимков, каждый корень привязан к git-коммиту)

**Для вашей команды безопасности:**
- «Докажи, что сервис A вызывает сервис B на этом коммите.» (доказательство Меркла, проверяемое офлайн)
- «Докажи, что этой зависимости НЕ существует.» (доказательство отсутствия через отсортированные листья)
- «Сгенерируй отчёт о соответствии.» (`knowing audit -proofs`, одна команда)
- «Читает ли этот пакет учётные данные и порождает ли процессы?» (`knowing audit-supply-chain --scan-all`)

---

## Цифры

| Что | Результат |
|---|---:|
| Межсистемный поиск | **P@10=0.330 холодный старт** (302 задачи, 17 репозиториев, 8 языков) |
| По сравнению с конкурентами | 3.79x codegraph (19K звёзд), 6.00x GitNexus, 6.35x Gortex, 22.0x grep |
| Классы эквивалентности | 277 отобранных вручную + усвоенных из использования, связывающих лексику с символами (+57% P@10) |
| Подавление шума | Неявная обратная связь по кластерам: R@10 +5.2%, MRR +12.6% (Django, 5 раундов) |
| Сэкономленные вызовы инструментов | На 47% меньше (один вызов контекста заменяет повторяющиеся grep+read) |
| Экономия токенов | На 84% меньше токенов (проводной формат GCF) |
| Скорость повторного запроса | В 93x быстрее (кеш подграфа по ключу Меркла) |
| Diff по Меркла | В 517x быстрее полного сканирования рёбер при 100K рёбрах |
| Охват тестов | Точность 98%, полнота 82% |
| Проверка целостности графа | 98 мс (24 936 рёбер) |
| Генерация доказательства | 72 мкс генерация, 1.2 мкс проверка |
| Истечение обратной связи | 100% истекают при изменении кода, накладные расходы 11% |
| Пропускная способность индексации | 16 репозиториев (8 языков) за ~60 с |
| Покрытие языков | 16/16 репозиториев проходят (Go, Python, TS, Rust, Java, C#, Ruby, мультиязычные) |
| Типы рёбер | 38 (включая цепочку поставок: reads_env, executes_process) |

Все бенчмарки воспроизводимы. Межсистемный бенчмарк (P@10=0.330) использует 17 репозиториев, закреплённых на точных коммитах, с [манифестом корпуса](bench/cross-system/corpus/MANIFEST.yaml) и [скриптом настройки](bench/cross-system/corpus/corpus-setup.sh) для полного воспроизведения с нуля. Детали протокола см. в [METHODOLOGY.md](bench/cross-system/METHODOLOGY.md).

---

## Быстрый старт

### Путь A: сервер MCP (рекомендуется для AI-агентов)

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

### Путь B: использование CLI (исследуйте граф сами)

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

### Проверьте вашу настройку

После индексации выполните эти команды, чтобы убедиться, что всё работает:

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

Если `knowing stats` показывает ноль узлов или очень мало рёбер, см.
[Устранение неполадок](docs/guide/cli.md#troubleshooting) ниже.

### Дополнительные команды CLI

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

Полный справочник команд см. в [Справочнике CLI](docs/guide/cli.md).

### Интеграция MCP

Добавьте сервер MCP к вашему агенту. Конфигурация одинакова везде; различается только путь к файлу.

| Агент | Файл конфигурации |
|-------|-------------|
| **Claude Code** | `.mcp.json` (корень проекта) или `~/.claude/mcp.json` (глобально) |
| **Cursor** | `.cursor/mcp.json` |
| **Windsurf** | `~/.codeium/windsurf/mcp_config.json` |
| **VS Code** (Copilot, Continue, Cline, Roo) | `.vscode/mcp.json` |
| **Zed** | `~/.config/zed/settings.json` в разделе `"context_servers"` |
| **Codex** (OpenAI) | `codex.json` или флаг `--mcp-config` |
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

Флаг `--watch` переиндексирует при изменении файлов. Ваш агент всегда запрашивает свежие данные. Не нужно вручную выполнять `knowing index` или указывать путь к базе данных: сервер MCP автоматически индексирует git-репозиторий при первом запуске и регистрирует его в реестре для будущих сессий.

Эмбеддинги отключены по умолчанию (подтверждено как нейтральное на бенчмарках холодного старта). Используйте `--embeddings`, чтобы включить их для экспериментов. Качество поиска обеспечивают структура графа и классы эквивалентности.

**Что получает ваш агент:** Ключевой инструмент — `context_for_task`. Когда ваш агент вызывает его с описанием задачи, knowing возвращает ранжированные, релевантные символы кода, упакованные в бюджет токенов. Это заменяет циклы grep-read. Другие полезные инструменты: `blast_radius` (что сломается, если я это изменю?), `test_scope` (какие тесты запускать?), `explain_symbol` (почему он оказался здесь в ранжировании?). Все 28 инструментов см. в [Справочнике инструментов MCP](docs/guide/mcp-tools.md).

**Проверьте, что это работает:**

1. Начните сессию с вашим агентом
2. Спросите: *«Используй инструмент context_for_task, чтобы найти символы, связанные с [чем-то конкретным в вашем коде]»*
3. Вы должны увидеть ранжированные символы с оценками и путями к файлам из вашей кодовой базы

Если результаты пусты: репозиторий, возможно, ещё индексируется (10-30 секунд при первом запуске). Если результаты кажутся нерелевантными: используйте конкретные имена символов в описании задачи (например, "find the `AuthMiddleware` handler", а не "find auth code"). Также можно проверить из CLI:

```bash
knowing stats          # should show nodes and edges
knowing query "MyFunc" # should find symbols you recognize
```

Для транспорта HTTP (мультиагентный режим, режим демона):

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

## Почему это работает

**Git версионирует файлы. knowing версионирует понимание кода.**

Вся система построена на одной идее: контентно-адресуемая идентичность. Каждый символ, связь и снимок хешируются по SHA-256. Этот единственный выбор даёт вам:

- **Обнаружение устаревания бесплатно.** Изменённый файл = новый хеш = устаревшие рёбра известны без сканирования.
- **Кеширование бесплатно.** Тот же корень пакета = те же результаты. Ускорение в 93x на неизменённых запросах.
- **Целостность бесплатно.** Проверьте все сохранённые хеши и непрерывность цепочки снимков. 98 мс.
- **История бесплатно.** Каждый снимок — это корень Меркла, привязанный к git-коммиту. Пройдите по цепочке.
- **Истечение обратной связи бесплатно.** Обратная связь хранит корень Меркла пакета. Изменения кода = изменение корня = старая обратная связь невидима.
- **Доказательства бесплатно.** Путь Меркла от листа к корню — это самодостаточное криптографическое доказательство.

| | Git | knowing |
|---|---|---|
| Что оно версионирует | Содержимое файлов | Связи кода и их смысл |
| Единица хранения | blob | node + edge + provenance + confidence |
| Идентичность | `sha256(content)` | `sha256("node\0" + repo + package + name + kind)` |
| Снимок | дерево blob'ов | Иерархический Меркла: repo -> package -> edge-type -> leaf |
| Diff | Какие строки изменились | Какие пакеты изменились, что сломалось, что нового |
| История | Как выглядел код | Что кодовая база понимала о самой себе |

---

## Как это работает

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

Две плоскости:
- **Исполнение:** индексирует репозитории, извлекает символы и связи, принимает трейсы, хранит снимки.
- **Интеллект:** вычисляет радиус поражения, пакеты контекста, охват тестов, обратную связь, сообщества из сохранённого графа.

Граница важна: интеллектуальные функции читают граф и производят производные результаты. Они не могут исказить факты графа. Плохое ранжирование даёт плохую рекомендацию; но оно не может сделать доказательство недействительным.

---

## Возможности

### Языки и форматы

| Язык/Формат | Экстрактор | Обнаружение фреймворков/паттернов |
|---|---|---|
| Go | tree-sitter + `go/packages` + SCIP | net/http, gin, echo, chi, gorilla/mux |
| TypeScript/JavaScript | tree-sitter | Express.js, Fastify, Hono, NestJS, Next.js |
| Python | tree-sitter | Flask, FastAPI, Django |
| Rust | tree-sitter | Actix, Axum, Rocket |
| Java | tree-sitter | Аннотации Spring |
| C# | tree-sitter | Атрибуты ASP.NET |
| Protocol Buffers | tree-sitter | объявления service, message, enum, RPC |
| Terraform (HCL) | tree-sitter | объявления resource, data, module, variable |
| SQL | tree-sitter | таблицы, представления, функции, процедуры, рёбра FK |
| Kubernetes YAML | yaml.v3 | deployments, services, configmaps, рёбра label-selector |
| CloudFormation/SAM | yaml.v3 | resources, перекрёстные ссылки !Ref/!GetAtt/!Sub |
| Docker Compose | yaml.v3 | services, ports, networks, связи depends_on |
| GitHub Actions | yaml.v3 | workflows, jobs, steps, ссылки на action |
| Serverless Framework | yaml.v3 | functions, events, ссылки на resource |
| CSS/SCSS | tree-sitter | селекторы, кастомные свойства, зависимости var() |
| Паттерны Event/MQ | multi-language | публикация/подписка Kafka, NATS, SQS, RabbitMQ |
| OpenAPI/JSON Schema | json/yaml | endpoints, модели, разрешение $ref |
| Dockerfile | parser | базовые образы FROM, многоэтапные зависимости COPY --from, порты EXPOSE |
| Makefile | parser | зависимости target, директивы include, ссылки на variable |
| Helm Charts | yaml.v3 | зависимости chart, ссылки на template, инъекция values |
| GitLab CI | yaml.v3 | job needs, шаблоны extends, файлы include, artifacts |
| package.json (npm) | json | dependencies, devDependencies, peerDependencies, scripts |
| GraphQL | parser | определения типов, ссылки на типы полей, реализации interface |
| Ruby | tree-sitter | классы, модули, определения методов, рёбра require |
| .env файлы | parser | объявления переменных окружения, межфайловые ссылки |

Все экстракторы срабатывают пофайлово через мультидиспетчеризацию; результаты объединяются. Tree-sitter производит рёбра с уверенностью 0.7 (`ast_inferred`); `go/packages` и SCIP — с 0.95-1.0 (`ast_resolved`, `scip_resolved`).

### Инструменты MCP

| Инструмент | Назначение |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | Построение и инспекция графа |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | Понимание влияния и путей |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | Сравнение состояний графа и ревью изменений |
| `runtime_traffic`, `dead_routes`, `trace_stats` | Запрос связей, наблюдаемых во время выполнения |
| `context_for_task`, `context_for_files`, `context_for_pr`, `explain_symbol` | Ранжированный контекст для агентов |
| `ownership`, `ownership_query`, `test_scope`, `communities`, `plan_turn`, `feedback` | Маршрутизация работы, запрос владельцев/авторов кода, выбор тестов, улучшение ранжирования |
| `prove`, `prove_absent`, `fsck` | Криптографические доказательства, доказательства отсутствия, проверка целостности |
| `untrack_repo` | Удаление всех данных репозитория (узлы, рёбра, файлы, снимки, обратная связь, память задач, заметки графа) |

Промпты MCP: `refactor_safely`, `review_pr`, `investigate_dead_code`.

### Ресурсы MCP

8 ресурсов только для чтения для ориентации агента без вызова инструмента:

| Ресурс | Что он возвращает |
|---|---|
| `knowing://report` | Размер графа, топ видов, число горячих точек, возраст снимка |
| `knowing://schema` | Виды узлов, типы рёбер, уровни происхождения, формат хеша |
| `knowing://stats` | Счётчики по репозиториям, видам и типам рёбер |
| `knowing://repos` | Все отслеживаемые репозитории со счётчиками и временем последней индексации |
| `knowing://session` | Вызовы контекста, обслуженные символы, попадания/промахи кеша, время работы |
| `knowing://index-health` | Статус здоровья/устаревания/повреждения, проверка целостности |
| `knowing://communities` | Список сообществ с их сплочённостью и корнями Меркла |
| `knowing://community/{id}` | Детали отдельного сообщества (шаблон ресурса) |

---

## Проводные форматы

| Формат | Назначение | Экономия относительно JSON |
|---|---|---|
| **GCF** (Graph Compact Format) | Потребление LLM: построчный, позиционные поля | На 84% меньше токенов |
| **GCB** (Graph Compact Binary) | Транспорт сервисов и кеширование: varint, с префиксом длины | На 74% меньше байтов |
| **JSON** | Отладка человеком, универсальные потребители | Базовый уровень |

GCF использует поля, разделённые `|`, и локальные ID (`$1 -> $3`) вместо повторяющихся полных имён. Разбирается LLM'ами, вмещая при этом в 5x больше контекста графа в тот же бюджет токенов. Дедупликация с сохранением состояния сессии сокращает повторяющиеся символы на 47%.

---

## Текущие ограничения

- Статический радиус поражения следует по рёбрам `calls`; другие типы рёбер дают контекст, а не обход.
- Рантайм-инструменты требуют приёма трейсов OpenTelemetry; без трейсов у них нет наблюдений.
- Обогащение LSP: Go, TypeScript, Python, Rust, Java, C#. Автоопределяется по маркерам проекта. Остальные откатываются на tree-sitter.
- Эмбеддинги отключены по умолчанию (подтверждено как нейтральное на бенчмарках холодного старта, сессия 23). Используйте `--embeddings`, чтобы включить.

---

## Документация

| Документ | Содержание |
|---|---|
| [Введение](docs/guide/introduction.md) | Как это работает, разбор конвейера поиска, 5-минутное знакомство |
| [Архитектура](docs/architecture/) | Дизайн системы, схемы, контентная адресация, модель демона |
| [Функции](docs/guide/features.md) | Инвентарь реализации, точки входа, ограничения |
| [Аудит и соответствие](docs/guide/audit-compliance.md) | Доказательства Меркла, fsck, цепочка снимков, гейты CI |
| [Справочник CLI](docs/guide/cli.md) | Команды, флаги, примеры, [устранение неполадок](docs/guide/cli.md#troubleshooting) |
| [Инструменты MCP](docs/guide/mcp-tools.md) | Схемы инструментов, параметры, форматы возврата |
| [Типы рёбер](docs/architecture/edge-types.md) | Семантика связей и происхождение |
| [Упаковка контекста](docs/architecture/context-packing.md) | RWR, HITS, ранжирование, бюджетирование токенов |
| [Ре-ранкер эмбеддингов](docs/architecture/embedding-reranker.md) | Локальный инференс, кеш векторов, профиль задержки |
| [Рантайм-трейсы](docs/operations/runtime-traces.md) | Приём OTel и рантайм-уверенность |
| [Проводные форматы](docs/architecture/wire-formats.md) | Форматы GCF, GCB, JSON и бенчмарки |
| [Дорожная карта](docs/roadmap.md) | Завершённые рабочие потоки и следующие приоритеты |
| [Бенчмарки](bench/README.md) | Воспроизводимые бенчмарки ценности с контрактами производительности |
| [Исследование](docs/research/content-addressing-as-computation-primitive.md) | Тезис, на котором построен knowing: контентная адресация как примитив вычислений ([DOI: 10.5281/zenodo.20342255](https://zenodo.org/records/20342255)) |
| [Hooks](hooks/README.md) | Интеграция хуков Claude Code |

## Лицензия

Apache-2.0 (c) 2026 Dayna Blackwell / Blackwell Systems. См. LICENSE и NOTICE.
