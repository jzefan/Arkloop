# Spike S1 — PDF 解析可行性评估

> **Status**: ready to run | **Owner**: jzefan | **Time budget**: 1-2 工作日 | **Blocks**: M1.1 (PDF parser in `shared/bookparser`)
>
> Refer to `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md` for the S1 context.

## 目的

在 M1.1 投入 PDF 解析模块之前，**用真实中文教材**评估 PyMuPDF + Tesseract 这套开源工具链的输出质量：

1. 文本提取顺序在多栏排版下是否正确
2. heading 识别（基于字号/字体启发式）的准确率
3. 扫描页 OCR 的中文准确率
4. 图片/表格的抽出率
5. 公式抽取是否可行

输出**结论**写回 `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md` 的 "Spike S1" 小节，告诉 M1.1 实施者：
- Tesseract 中文够不够，还是需要切换到 PaddleOCR
- 公式抽取是否纳入 M1.1 范围
- heading 启发式的兜底（design 文档已写默认方案：`heading_inferred=false` 占比 >50% 时退化为纯序列 chunks）

## 怎么跑

### 准备

1. **教材**：一本你手上真实的中文 PDF 教材。包含以下元素**优先**：
   - 多栏排版的正文（验证文本顺序）
   - 至少 3 级章节标题（验证 heading 启发式）
   - 至少一页扫描页（验证 OCR；扫描页文本是图片，PyMuPDF 抽不出）
   - 图片 + 图注、表格、公式

2. **依赖**：

```bash
# 在 sandbox 容器内安装：
pip install pymupdf==1.24.* pdfplumber==0.11.* pillow==10.* pytesseract==0.3.*
# Tesseract 二进制 + 中文语言包：
apt-get install tesseract-ocr tesseract-ocr-chi-sim
```

或使用项目现有 sandbox image（如果它包含上述）；查 `compose.yaml` 的 `sandbox-agent-image` 是否已带 PyMuPDF。

### 跑解析

```bash
cd tools/spike-s1
python parse_book.py /path/to/textbook.pdf --out /tmp/parsed.json
```

输出 JSON 结构（节选）：

```json
{
  "meta": {
    "page_count": 320,
    "total_blocks": 1245,
    "ocr_pages": [42, 43, 187],
    "elapsed_s": 47.3
  },
  "blocks": [
    {"type": "heading", "text": "第三章 物理光学", "page": 78, "heading_inferred": true, "heading_confidence": 0.91, "font_size": 18.0},
    {"type": "paragraph", "text": "光的干涉是...", "page": 78, "heading_path": ["第三章 物理光学"]},
    {"type": "image", "page": 79, "caption": "图 3.1 杨氏双缝实验", "ocr_text": "..."},
    {"type": "table", "page": 80, "text_markdown": "| 实验 | ... |\n|---|---|\n", "rows": 5, "cols": 3}
  ]
}
```

### 评分

```bash
python evaluate.py /tmp/parsed.json /path/to/textbook.pdf --rubric rubric.yaml
```

`rubric.yaml` 让你针对每个采样章节/页人工打分；脚本聚合并输出：

```
=== Spike S1 Evaluation Report ===
File: 大学物理（上）.pdf (320 pages, ~32MB)

[Heading detection]
  Sampled 20 chapter/section headings.
  - Correctly identified as heading:     17 / 20 = 85%
  - Correctly assigned heading_path:     15 / 20 = 75%
  - False positives (page numbers etc.): 3 instances in 20 pages

[Text extraction order]
  Sampled 5 multi-column spread pages.
  - Reading order correct:               3 / 5 = 60%
  Note: 2-column physics pages fine; 3-column glossary appendix scrambled.

[OCR (scanned pages)]
  Sampled 1 scanned page (180 chars 估计).
  - Char accuracy (manual diff):         142 / 180 = 79%
  - Conclusion: Tesseract chi-sim adequate for backup, NOT primary OCR.

[Image / table / formula]
  - Images extracted with caption:       12 / 14 = 86%
  - Tables extracted (markdown round):   2 / 3 = 67%
  - Formulas: PyMuPDF extracts as text "Δy = λL/d" intact; no LaTeX form.

[Elapsed]
  47.3s for 320 pages = 6.8 pages/s

=== Recommendation ===
- Tesseract OCR quality marginal for scanned content; recommend
  evaluating PaddleOCR (paddleocr-chinese) before M1.1 commits.
- Multi-column reading-order failures suggest a fallback: at chunk
  time, treat ambiguous-ordering pages as a single block per page.
- Formula → LaTeX is OUT of M1.1 scope; ship "formula chunks as text"
  per design doc §"M1 implementation 细则".
```

### 把结论回填到 design 文档

完成后编辑 `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md` 的 **"Spike S1：PDF 解析可行性"** 小节，把上面 Recommendation 拷过去，并加日期戳。

## 文件

- `parse_book.py` — PyMuPDF + pdfplumber + Tesseract 的解析驱动；产出统一 JSON
- `evaluate.py` — 加载 JSON + 人工评分 → 输出聚合报告
- `rubric.yaml` — 评估维度模板，跑前你按需打开/关闭某些维度
- `sample_outputs/` — 历史 spike 结果留档（runs 完后把输出归档到这里）

## 不在 spike 范围

- **真正实现 `shared/bookparser` 的 PDF 后端** —— 那是 M1.1 工作。spike 只 evaluate，不 ship。
- **自动化 metric** —— OCR 字准确率、heading recall 这些都是**人工抽样打分**，不需要 ground truth dataset。spike 是 go/no-go 决策，不是科研。
- **多本书的横向对比** —— 单本评估足够。如果第一本特别难（全扫描件 / 三栏复杂排版），换更典型的再跑一次。

## 完成判据

- `parse_book.py` 在你手上的真实教材上跑通，输出合法 JSON
- 人工抽样 20 个章节标题、5 个多栏页、1 个扫描页打分
- 聚合报告生成
- design 文档 S1 小节已回填结论
- 决策明确：Tesseract vs PaddleOCR、公式 in vs out
