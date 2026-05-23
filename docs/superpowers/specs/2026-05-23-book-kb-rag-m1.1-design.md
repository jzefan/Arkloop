# Design: book-kb-rag M1.1 — PDF / DOCX / Image 解析

> Status: ready-for-plan
> Owner: jzefan
> Created: 2026-05-23
> Companion specs: [M0 design](./2026-05-21-book-kb-rag-design.md) · [M1 decomposition](./2026-05-21-book-kb-rag-m1-decomposition-design.md) · [PRD](../../prd/book-kb-rag.md)
> Spike inputs: design 文档 "Spike S1：PDF 解析可行性" 一节 + `tools/spike-s1/sample_outputs/fuchan-10-eval.txt`

## Context

M1.0 已交付 `.txt` / `.md` 端到端摄入；M1.1 把 `bookparser.Parser` 的真实后端扩到 PDF / DOCX / 图片，使老师上传真书可用。

Spike S1（《妇产科学》第十版 511 页）给出的硬数据：

- **PyMuPDF 文本提取 + multi-column 顺序对中文医学教材直接可用**（多栏顺序 5/5 通过）
- **heading 字号判定 100% 命中**，但 **heading_path 当前 depth 算法（`(p90-size)//2`）对正文字号均匀的书彻底失效** — 必须改为 size + font-family 联合判定
- **图像检测会被 PDF 矢量装饰污染** — 必须 size 阈值过滤
- **OCR 不阻塞**：本次 spike 教材 0 扫描页，Tesseract chi_sim 保留作 fallback；遇真扫描书的实战调整推到 M1.2 之后
- **公式**：教材无样本，沿用 PRD 决策 `chunk_type=formula` 存纯文本，不进 LaTeX
- **吞吐 7.8 pages/s**：511 页教材 65s 完整解析；M1.1 接受这个时延作为预期

## 范围摘要

5-7 工作日交付：

- `bookparser` 新增 PDF、DOCX、图片三个后端，所有走 sandbox 调 Python（PyMuPDF / python-docx / Pillow + pytesseract）
- `MultiFormatParser`：按 mime 分派；保留 `TextOnlyParser` 作为 `.txt`/`.md` 后端
- Block.Type 真正用上 `image`、`table`、`formula`；chunker 配合每种类型保持独立 chunk
- heading 启发式重写：**font-size + font-family** 联合判定；同字体族内按字号排序定义层级
- Image / table block 携带 `metadata` 富信息（caption 候选、bbox、原始 URI 引用）
- console-lite KB 详情页：document 状态 panel 新增 `block_type` 分布饼图（验证图像/表格被识别）
- 测试：fixture-driven（不依赖真实 PDF）覆盖 chunker + parser 集成路径

## 数据模型

**无新增表 / 无新增迁移**。复用 M1.0 schema：

- `kb_chunks.chunk_type` 字段已存（M1.0 写死 `paragraph`）；M1.1 起写入真实的 `paragraph / heading / image / table / formula`
- `kb_chunks.metadata` JSONB 字段已存（M1.0 写 `{}`）；M1.1 起写入 block 元信息（image 加 `caption_candidate`、`bbox`；table 加 `rows`、`cols`、`markdown`；heading 加 `font_family`、`font_size`、`level`）
- `kb_documents.parse_meta_json` 已存；M1.1 起写入 `{page_count, block_type_counts, heading_inferred_ratio, scanned_pages, parser_version, elapsed_ms}`

## Sandbox Python 调用契约

bookparser 的 PDF/DOCX/image 后端**不在 Go 侧实现解析逻辑**。Worker 拿到 blob 后，通过现有 `sandbox-agent` 起一个 Python 子进程跑统一脚本 `bookparser_runner.py`（新增到 `src/services/shared/bookparser/sandbox/`），脚本输出统一 JSON 到 stdout，Go 侧解码到 `ParsedDoc`。

理由：

1. PDF/DOCX 生态在 Python 远成熟于 Go，不发明 Go 端口
2. 复用现有 sandbox 隔离机制，不引新进程模型
3. Go 接口（`Parser.Parse`）稳定，解析后端独立迭代/换实现不动调用方

### Python 端契约

**输入**（命令行 + stdin 二进制流）：

```bash
bookparser_runner.py --mime application/pdf --max-pages 0 < blob.bin > out.json
```

支持的 `--mime`：
- `application/pdf` → PyMuPDF + pdfplumber + pytesseract（fallback OCR）
- `application/vnd.openxmlformats-officedocument.wordprocessingml.document` → python-docx
- `image/png` / `image/jpeg` / `image/webp` → Pillow + pytesseract（整图 OCR 为单个 paragraph block，附带 image block）

**输出 JSON**（与 Go `ParsedDoc` 直接对应）：

```json
{
  "meta": {
    "source_mime": "application/pdf",
    "page_count": 511,
    "block_type_counts": {"paragraph": 19736, "heading": 514, "image": 1100, "table": 217},
    "heading_inferred_ratio": 1.0,
    "scanned_pages": [],
    "parser_version": "m1.1-2026-05-23",
    "elapsed_ms": 65100
  },
  "blocks": [
    {"type": "heading", "text": "第二章 女性生殖系统发育与解剖", "metadata": {"page": 29, "font_family": "FZLTZCHK--GBK1-0", "font_size": 21.0, "level": 1, "heading_inferred": true, "heading_confidence": 1.0}},
    {"type": "paragraph", "text": "本章数字资源...", "metadata": {"page": 29, "heading_path": ["第二章 女性生殖系统发育与解剖"]}},
    {"type": "image", "text": "", "metadata": {"page": 32, "bbox": [120, 200, 480, 460], "caption_candidate": "图 2-1 女性生殖器官", "ocr_text": "...", "asset_size_bytes": 87432}},
    {"type": "table", "text": "| ... |", "metadata": {"page": 373, "rows": 5, "cols": 4, "markdown": "| ... |"}}
  ]
}
```

### Image 过滤规则（spike 强约束）

Python 端**必须丢弃**满足以下任一条件的 image 候选：

- `width < 32 px` 或 `height < 32 px`
- `asset_size_bytes < 4096`
- 整页 image 数 > 50（页级阈值；超过认为是矢量装饰污染，整页 image block 全部丢弃，但保留 paragraph）

阈值确切数字写进脚本顶部常量，便于以后调；spike 数据下，3 个污染页（封面、TOC、底页）单页报 309-4146 张"image"，正文页只有 1-5 张 — 阈值 50 足以隔离。

### Heading 层级判定（spike 强约束）

按"在该文档中观测到的所有非正文字体的相对大小"分层，**不与 body p90 比较**：

1. 第一遍：扫所有 span，按 `(font_family, round(font_size, 1))` 分桶统计 span 数
2. 桶按 `font_size` 降序排，按 span 数从少到多取前 N 个作为 heading 候选（N=4 覆盖到 L4 章节）
3. 桶之间的 size 间隔 < 1pt 视为同级
4. 一个 span 命中某 heading 桶则 `level = 桶序号`（1=最大）
5. heading_stack 按 level 维护：进入 level L 时截断到 `[:L-1]` 后 append

落地：fuchan-10 的 chapter 21pt + section 13pt 会进同字体族两桶 → level 1/2，path 正确。

### Sandbox 调用层（Go 侧 `bookparser/sandbox.go`）

```go
type SandboxParser struct {
    runner SandboxRunner // 注入；生产实现包装 sandbox-agent HTTP client
}

func (p *SandboxParser) Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error)
```

`SandboxRunner` 接口对调用方稳定；测试用 fake runner（喂固定 JSON），不依赖 Python/sandbox 起得来。

## MultiFormatParser

新增 `multiformat.go`：

```go
type MultiFormatParser struct {
    text    *TextOnlyParser
    sandbox *SandboxParser // PDF / DOCX / image
}

func NewMultiFormatParser(sandbox SandboxRunner) *MultiFormatParser

func (m *MultiFormatParser) Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error) {
    base := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
    switch base {
    case "text/plain", "text/markdown":
        return m.text.Parse(ctx, r, mime)
    case "application/pdf",
         "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
         "image/png", "image/jpeg", "image/webp":
        return m.sandbox.Parse(ctx, r, mime)
    default:
        return ParsedDoc{}, fmt.Errorf("%w: %s", ErrUnsupportedMime, mime)
    }
}
```

`worker/internal/kbingest/processor.go` 把 `NewTextOnlyParser()` 替换为 `NewMultiFormatParser(sandboxRunner)`；其他业务流程不动。

## Chunker 集成

M1.0 的 `bookchunker.Chunk(ParsedDoc, opts)` 已经按 BlockType 处理；M1.1 不改 Chunker 签名，**只补**：

- `BlockImage` / `BlockTable` / `BlockFormula` 在 chunker 中保持独立 chunk（不与相邻文本合并），M1.0 既有逻辑
- 新增：若整份文档 `heading_inferred_ratio < 0.5`，chunker 强制把所有 `heading_path` 重置为空（PRD "兜底"约定）— 这是新规则，需要在 chunker 层加 fixture 测试
- `Chunk.Metadata` 透传 Block.Metadata（M1.0 只透 `chunk_type`），让 kbapi search 响应能带 caption / table dims / page 给前端

## API & 前端

**API**：无新端点。`kb_search` 响应已有 `chunk_type` 字段（M1.0），M1.1 起会返回真正的 `image / table / formula` 类型；前端按类型不同渲染（详见下）。

**console-lite KB 详情页**：

- Document 状态卡片显示 `block_type_counts`（M1.1 写到 `parse_meta`），bar chart 形式：`heading: 514, paragraph: 19736, image: 1100, table: 217`
- Document 失败时显示 `error_message` 全文（M1.0 已有）
- Search 调试框结果项按 chunk_type 加图标（📷 image、▦ table、🔣 formula）

**book-tutor-agent prompt**：扩 1 段说明 — 命中 `image / table` 类型 chunk 时引用方式（"如图 X 所示" / "如下表所示"），让老师看到 RAG 也能引图表内容。

## 测试

### Fixture-driven（不依赖真 PDF）

`bookparser/sandbox_test.go`：fake SandboxRunner 喂 4 个固定 JSON（PDF 章节带 image+table、image-only PDF 触发 Tesseract OCR、DOCX 简单段落、坏 JSON），验证 Go 侧解码 + 字段映射。

`bookchunker/chunker_test.go`（M1.0 既有）扩 3 个表驱动 case：
- ParsedDoc 含 `image` block → 输出对应 chunk `chunk_type=image`，独立成 chunk
- ParsedDoc 含 `table` block → 同上
- ParsedDoc 80% blocks `heading_inferred=false` → 输出所有 chunks `heading_path=[]`

### Python 端单元（pytest，可选 M1.1.1）

`bookparser/sandbox/test_runner.py`：3 个迷你 fixture PDF（手工 0.5 页生成），覆盖 image 阈值过滤、heading 字体分桶、scanned page OCR fallback。Python 端测试不进 Go test suite；CI 单独 job 跑（或留到 M1.1.1 reaching out 实测）。

### Integration smoke（要求真 sandbox 起来）

`worker/internal/kbingest/sandbox_integration_test.go`（`-tags=sandbox_integration`）：把 `tools/spike-s1/sample_outputs/` 的 mini PDF 喂进 pipeline，断言：
- document 状态最终 `ready`
- chunks 表有 `chunk_type=image` 至少 1 条
- `parse_meta_json.block_type_counts.image > 0`

默认 CI 不跑（需要装 PyMuPDF/Tesseract）；本地 + nightly 跑。

## M1.1 验收

| Check | 验证方式 |
| --- | --- |
| `MultiFormatParser` 编译，`text/plain` 与 PDF 走不同分支 | Go unit test |
| 上传 spike-s1 教材 PDF（fuchan-10.pdf 或同类）后状态 `ready` | UI 实测 |
| 该文档对应 `kb_chunks` 至少出现 `paragraph / heading / image / table` 4 种类型 | SQL: `SELECT DISTINCT chunk_type FROM kb_chunks WHERE document_id=…` |
| `kb_documents.parse_meta_json` 写入 `block_type_counts` + `parser_version` | SQL 查询 |
| `kb_search` 返回结果项 `chunk_type` 准确 | curl + 前端肉眼 |
| Heading 字体 + 字号联合判定后，章节 + 子节 `heading_path` ≥ depth 2 命中率 ≥ 70% | 对该 PDF 重跑 `tools/spike-s1/evaluate.py`（heading 部分） |
| Image / table 独立成 chunk（不与 paragraph 合并） | chunker 单元测试 |
| `heading_inferred_ratio < 0.5` 触发 sequential chunks 兜底 | chunker fixture 测试 |
| 单文档 511 页解析 < 90s（spike 实测 65s 留余量） | worker 日志 `elapsed_ms` |
| console-lite KB 详情页显示 block_type 分布 | UI 截图 |
| 失败 case：DOCX 加密 / PDF 截断 / 不支持 mime → document `failed` + `error_message` 可读 | UI 实测 + integration test |

## 关键技术决策

1. **Python 解析走 sandbox 而非内置 Go 库** — 见 "Sandbox Python 调用契约" 理由
2. **Heading 用 font-family + 桶分级**，不用绝对字号阈值 — spike S1 证明绝对阈值对均匀正文字号失效
3. **OCR 始终用 Tesseract chi_sim**，不引 PaddleOCR — spike 0 扫描页不支持选 PaddleOCR；后续若 Tesseract 真不够再 spike 切换
4. **不做 LaTeX 公式转换** — 沿用 PRD 决策；公式以纯文本入库，老师看原文截图
5. **OCR 的 image-only 整页路径**：parser 端 `text==""` 时尝试 OCR，OCR 文本作为单个 `paragraph` block（不是 `image` block），原图作为独立 `image` block 并附 `ocr_text` metadata
6. **图像 size 阈值硬编码**（32px / 4KB / 50 per page）— 不做配置；后续真出问题再开关
7. **Python 进程隔离用现有 sandbox-agent**，不引新 worker pool — 与 Spike S1 的 README 指引一致
8. **chunker 兜底（heading_inferred_ratio < 0.5）由 chunker 实施**而非 parser — chunker 是纯函数易测，parser 是 IO 边界混 IO 难测

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| Sandbox image 不带 PyMuPDF/pdfplumber/tesseract → parser 启动时 ImportError | M1.1 Task 1 增加 sandbox image 依赖 + smoke check；启动期 Python 端遇 ImportError 返回结构化错误（"missing pymupdf"），Go 侧把 document 标 `failed` + error_message 直白指出缺哪个包 |
| spike-s1 之外的医学 / 法律 / 古文教材字体多样 → heading 桶算法不普适 | 接受 — heading 命中率给到 70% 已是 M1.1 验收线，剩下 30% 走 sequential chunks 兜底，不阻塞老师出题 |
| Sandbox HTTP 调用大文件超时（38MB PDF + 65s 解析） | Python 进程内不读全文到内存（PyMuPDF streaming）；sandbox-agent client 把 default timeout 提到 5min（worker 端配置） |
| pdfplumber 表格行列不准（spike 0/5） | 接受 — 表格内容（markdown 文本）入库就有价值；rows/cols 字段写入但不做语义保证；UI 只展示 markdown，不依赖 rows/cols |
| OCR 误识别引文 → 污染检索 | 接受 — Tesseract OCR text 走和正常文本一样的 embed 链路，相关性自然降级；不做特殊处理 |
| 巨型 PDF（>200MB / >2000 页）OOM | M1.1 加 `max_upload_bytes` 拒绝 > 100MB；> 1000 页给 warning 但不拒绝 |

## 不在 M1.1 范围（推到 M1.2+）

- 图片 caption 经多模态 LLM 生成（PRD 故事 11 的 "caption via multimodal"）— M1.1 只取 caption_candidate（图下方第一行短文本，靠 PDF layout 启发式）
- 公式转 LaTeX
- 真实大规模扫描教材 OCR 调优（包括切 PaddleOCR）
- EPUB / PPT / Markdown / HTML
- 跨 KB 检索
- chunking 策略调优（M1.0 默认 256-512 tokens + 40 重叠不动）

## 不在本设计范围

- M1.2 RAG 出题（QuestionStore / kb_draft_questions / 题库 UI）— 单独 design 文档
- M1.3 组卷（papercompose / kb_compose_paper / 试卷 UI）— 单独 design 文档
- M2 Linked exam 集成

## 下一步

1. 走 `superpowers:writing-plans` 生成 M1.1 实施 plan（预计 8-10 tasks，命名 `2026-05-23-book-kb-rag-m1.1.md`）
2. Plan 第 1 task：sandbox image 加 PyMuPDF / pdfplumber / pytesseract / python-docx 依赖 + smoke check
3. Plan 第 2 task：`bookparser_runner.py` + 3 个 mini fixture pytest
4. Plan 第 3-N task：Go 侧 SandboxParser / MultiFormatParser / kbingest 接线 / chunker 扩展 / console-lite UI / acceptance
