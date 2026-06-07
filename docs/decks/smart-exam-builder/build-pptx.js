// 智能组卷智能体 · 可编辑 PPTX 生成（瑞士国际主义 · IKB 克莱因蓝）
// run: NODE_PATH=$(npm root -g) node build-pptx.js
const P = require("pptxgenjs");

const C = {
  ikb: "002FA7",
  ink: "0A0A0A",
  paper: "FFFFFF",
  grey1: "F0F0EE",
  grey3: "737373",
  border: "D4D4D2",
  borderSubtle: "E0E0E0",
  white: "FFFFFF",
  white70: "BFC9EC", // 用于 IKB 底上的次级白（近似）
};
const F = { sans: "Arial", mono: "Consolas" };

const pres = new P();
pres.defineLayout({ name: "W", width: 13.333, height: 7.5 });
pres.layout = "W";
pres.author = "ArkLoop";
pres.title = "智能组卷智能体";
const W = 13.333, H = 7.5, M = 0.62;

// ---------- helpers ----------
function chrome(s, left, right, dark) {
  const col = dark ? C.white : C.grey3;
  s.addText(left, { x: M, y: 0.32, w: 8, h: 0.3, fontFace: F.mono, fontSize: 10.5, color: col, charSpacing: 2, transparency: dark ? 25 : 0 });
  s.addText(right, { x: W - M - 4, y: 0.32, w: 4, h: 0.3, fontFace: F.mono, fontSize: 10.5, color: col, charSpacing: 2, align: "right", transparency: dark ? 25 : 0 });
}
function kicker(s, t, x, y, dark) {
  s.addText(t, { x, y, w: 9, h: 0.3, fontFace: F.mono, fontSize: 11, bold: true, color: dark ? C.white : C.ikb, charSpacing: 3 });
}
function hairline(s, x, y, w, color) {
  s.addShape(pres.shapes.LINE, { x, y, w, h: 0, line: { color: color || C.borderSubtle, width: 1 } });
}
function title(s, t, x, y, w, size) {
  s.addText(t, { x, y, w: w || 9, h: size ? size / 50 : 1.0, fontFace: F.sans, fontSize: size || 40, color: C.ink, bold: false, charSpacing: 0, lineSpacingMultiple: 1.0 });
}

// ============ P1 · 封面 ============
let s = pres.addSlide();
s.background = { color: C.ikb };
chrome(s, "ARKLOOP · 录题 → 组卷流水线", "AI EXAM · 01 / 12", true);
kicker(s, "INTELLIGENT PAPER COMPOSITION · book-tutor-agent", M, 1.5, true);
s.addText([
  { text: "智能组卷", options: { breakLine: true } },
  { text: "智能体", options: { italic: true } },
], { x: M, y: 2.3, w: 11, h: 2.6, fontFace: F.sans, fontSize: 72, color: C.white, bold: false, lineSpacingMultiple: 1.0 });
hairline(s, M, 5.95, W - 2 * M, C.white);
s.addShape(pres.shapes.LINE, { x: M, y: 5.95, w: W - 2 * M, h: 0, line: { color: C.white, width: 1, transparency: 70 } });
s.addText("从教材知识库到可发布试卷 —— 稳定生成 A1–A4 国考级题型的端到端命题引擎。",
  { x: M, y: 6.1, w: 9.5, h: 0.7, fontFace: F.sans, fontSize: 15, color: C.white, transparency: 12 });
s.addText("ArkLoop · 2026", { x: W - M - 4, y: 6.15, w: 4, h: 0.3, fontFace: F.mono, fontSize: 10.5, color: C.white, align: "right", charSpacing: 2, transparency: 30 });

// ============ P2 · 一句话定位 ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "POSITIONING · 定位", "02 / 12");
kicker(s, "ONE LINE", M, 1.5);
s.addText([
  { text: "用 ", options: {} },
  { text: "教材知识库 + 种子题锚定 + 命题技能", options: { color: C.ikb, bold: true } },
  { text: " 三重约束，", options: { breakLine: true } },
  { text: "把大模型不稳定的「凭空出题」，", options: { breakLine: true } },
  { text: "收敛成 ", options: {} },
  { text: "可溯源、可发布", options: { color: C.ikb, bold: true } },
  { text: " 的标准化医学试卷。", options: {} },
], { x: M, y: 2.5, w: 11.6, h: 3.2, fontFace: F.sans, fontSize: 34, color: C.ink, lineSpacingMultiple: 1.18 });

// ============ P3 · 痛点 ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "THE PROBLEM · 痛点", "03 / 12");
kicker(s, "WHY IT EXISTS", M, 1.45);
s.addText([{ text: "为什么", options: { breakLine: true } }, { text: "需要它" }],
  { x: M, y: 1.95, w: 5.2, h: 2.2, fontFace: F.sans, fontSize: 52, color: C.ink, lineSpacingMultiple: 1.0 });
s.addText("高质量国考级 A1–A4 题稀缺，而通用大模型凭空出题，质量不稳、爱幻觉、难标准化。",
  { x: M, y: 4.6, w: 5.0, h: 1.4, fontFace: F.sans, fontSize: 15, color: C.grey3, lineSpacingMultiple: 1.35 });
const probs = [
  ["01", "高质量题稀缺", "资深命题专家产能有限，人工出题慢", false],
  ["02", "质量不稳 · 爱幻觉", "伪 A2 泛滥：病例外壳套教材题", false],
  ["03", "组卷难标准化", "题型 / 难度 / 知识点分布靠人工配比", false],
  ["04", "题目无溯源", "老师不敢直接发给考生", true],
];
let py = 1.7;
const colX = 6.6, colW = 6.1;
probs.forEach(([n, t, d, accent]) => {
  hairline(s, colX, py, colW, accent ? C.ikb : C.borderSubtle);
  if (accent) s.addShape(pres.shapes.LINE, { x: colX, y: py, w: colW, h: 0, line: { color: C.ikb, width: 2 } });
  s.addText(n, { x: colX, y: py + 0.18, w: 0.7, h: 0.5, fontFace: F.mono, fontSize: 13, bold: true, color: accent ? C.ikb : C.grey3, charSpacing: 2 });
  s.addText(t, { x: colX + 0.85, y: py + 0.15, w: colW - 0.9, h: 0.4, fontFace: F.sans, fontSize: 17, bold: true, color: C.ink });
  s.addText(d, { x: colX + 0.85, y: py + 0.58, w: colW - 0.9, h: 0.4, fontFace: F.sans, fontSize: 13, color: C.grey3 });
  py += 1.28;
});

// ============ P4 · 三段式 ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "PIPELINE · 整体流程", "04 / 12");
kicker(s, "END TO END · 三段式", M, 1.4);
title(s, "三段式端到端", M, 1.85, 9, 44);
const stages = [
  { bg: C.grey1, fg: C.ink, sub: C.grey3, nb: "STAGE 01", t: "创建知识库", d: "教材分块 → Doubao 向量化 → pgvector 入库 → 绑定 exam 范围（专业/方向/主知识点）", tag: "STANDALONE / LINKED" },
  { bg: C.ink, fg: C.white, sub: "BFBFBF", nb: "STAGE 02", t: "创建种子题库", d: "录题助手解析教材目录 → 知识点树；RAG 检索教材 + 参考题，生成带溯源草稿", tag: "命题锚点 · SEED BANK" },
  { bg: C.ikb, fg: C.white, sub: "C7D2F2", nb: "STAGE 03", t: "构建医学试卷", d: "命题技能驱动 A1–A4 → pattern 强校验 → 组卷 → 写回 exam 发布给考生", tag: "A1 · A2 · A3 · A4" },
];
const cardY = 3.05, cardH = 3.75, gap = 0.3;
const cardW = (W - 2 * M - 2 * gap) / 3;
stages.forEach((st, i) => {
  const cx = M + i * (cardW + gap);
  s.addShape(pres.shapes.RECTANGLE, { x: cx, y: cardY, w: cardW, h: cardH, fill: { color: st.bg }, line: { type: "none" } });
  s.addText(st.nb, { x: cx + 0.3, y: cardY + 0.3, w: cardW - 0.6, h: 0.3, fontFace: F.mono, fontSize: 11, color: st.sub, charSpacing: 2 });
  s.addText(st.t, { x: cx + 0.3, y: cardY + 1.7, w: cardW - 0.6, h: 0.5, fontFace: F.sans, fontSize: 22, bold: true, color: st.fg });
  s.addText(st.d, { x: cx + 0.3, y: cardY + 2.25, w: cardW - 0.6, h: 1.0, fontFace: F.sans, fontSize: 12.5, color: st.fg, transparency: i === 0 ? 0 : 10, lineSpacingMultiple: 1.25 });
  s.addText(st.tag, { x: cx + 0.3, y: cardY + cardH - 0.5, w: cardW - 0.6, h: 0.3, fontFace: F.mono, fontSize: 10, color: st.sub, charSpacing: 2 });
});

// ============ P5 · 端到端数据流（时间线） ============
s = pres.addSlide();
s.background = { color: C.grey1 };
chrome(s, "DATA FLOW · 数据流", "05 / 12");
kicker(s, "TEACHER → CANDIDATE", M, 1.4);
title(s, "一条对话内的数据流", M, 1.85, 10, 40);
const steps = [
  ["INGEST", "教材导入", "PDF/Excel/图片", false],
  ["EMBED", "向量化", "Doubao Ark + pgvector", false],
  ["SEED", "种子题库", "录题 / RAG 草拟", false],
  ["GENERATE", "命题生成", "SKILL.md · A1–A4", false],
  ["PUBLISH", "组卷发布", "写回 exam → 考生", true],
];
const axisY = 4.6;
hairline(s, M + 0.4, axisY, W - 2 * M - 0.8, C.grey3);
const n = steps.length;
const slotW = (W - 2 * M) / n;
steps.forEach(([yr, name, desc, accent], i) => {
  const cx = M + slotW * i + slotW / 2;
  s.addShape(pres.shapes.OVAL, { x: cx - 0.08, y: axisY - 0.08, w: 0.16, h: 0.16, fill: { color: accent ? C.ikb : C.ink }, line: { type: "none" } });
  const up = i % 2 === 0;
  const ly = up ? axisY - 1.55 : axisY + 0.35;
  s.addText(yr, { x: cx - slotW / 2, y: ly, w: slotW, h: 0.3, fontFace: F.mono, fontSize: 10.5, bold: true, color: accent ? C.ikb : C.grey3, align: "center", charSpacing: 1.5 });
  s.addText(name, { x: cx - slotW / 2, y: ly + 0.32, w: slotW, h: 0.35, fontFace: F.sans, fontSize: 16, bold: true, color: accent ? C.ikb : C.ink, align: "center" });
  s.addText(desc, { x: cx - slotW / 2, y: ly + 0.72, w: slotW, h: 0.4, fontFace: F.sans, fontSize: 11.5, color: C.grey3, align: "center" });
});

// ============ P6 · 六大创新点 ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "INNOVATION · 创新点", "06 / 12");
kicker(s, "SIX CONSTRAINTS", M, 1.4);
title(s, "六重约束，收敛输出", M, 1.8, 10, 38);
const inno = [
  ["01", "种子锚定 + 技能驱动", "无种子 / 无命题技能不出题，把自由生成改成有参照的再生产", false],
  ["02", "pattern_tag 三层强校验", "build 提取 → 预览标红 → save 预检拒绝；A2 进 A2 出，绝不静默修正", true],
  ["03", "RAG 溯源命题", "每题挂教材原文 200–500 字快照，KB 重导后仍可回溯，防幻觉", false],
  ["04", "伪 A2 识别质检", "先建真实病例再抽考点，杜绝教材题套病例壳", false],
  ["05", "draft / confirm 两段式", "生成不写库、老师确认才落库，全程可中断可修改", false],
  ["06", "部分失败语义", "5 道里 4 道合格能单独保留，失败逐条报因", false],
];
const gY = 2.95, gGap = 0.28, gRows = 2, gCols = 3;
const gW = (W - 2 * M - (gCols - 1) * gGap) / gCols;
const gH = (H - gY - 0.55 - (gRows - 1) * gGap) / gRows;
inno.forEach(([nb, t, d, accent], i) => {
  const r = Math.floor(i / gCols), c = i % gCols;
  const cx = M + c * (gW + gGap), cy = gY + r * (gH + gGap);
  s.addShape(pres.shapes.RECTANGLE, { x: cx, y: cy, w: gW, h: gH, fill: { color: accent ? C.ikb : C.grey1 }, line: { type: "none" } });
  s.addText(nb, { x: cx + gW - 0.7, y: cy + 0.22, w: 0.5, h: 0.3, fontFace: F.mono, fontSize: 11, color: accent ? "C7D2F2" : C.grey3, align: "right", charSpacing: 1 });
  s.addText(t, { x: cx + 0.32, y: cy + 0.3, w: gW - 0.7, h: 0.55, fontFace: F.sans, fontSize: 16, bold: true, color: accent ? C.white : C.ink });
  s.addText(d, { x: cx + 0.32, y: cy + gH - 1.15, w: gW - 0.6, h: 1.0, fontFace: F.sans, fontSize: 12, color: accent ? C.white : C.grey3, transparency: accent ? 8 : 0, lineSpacingMultiple: 1.25 });
});

// ============ P7 · 好处 ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "BENEFITS · 好处", "07 / 12");
kicker(s, "FAST · STABLE · TRUSTED · STANDARD", M, 1.4);
title(s, "它带来什么", M, 1.8, 9, 38);
const bens = [
  ["SPEED · 快", "分钟级", "", "选种子 → 审核 → 发布，单批 ≤5 道小步快跑", true],
  ["STABLE · 稳", "0.3", "低温", "三重约束 + 低温度，输出稳定、风格一致", false],
  ["TRUSTED · 可信", "100%", "溯源", "题题溯源 + 伪 A2 质检，敢直接发给考生", false],
  ["STANDARD · 标准", "A1–A4", "", "20 / 40 / 20 / 20 国考配比一键约束，闭环发布", false],
];
const bY = 3.0, bGap = 0.3;
const bW = (W - 2 * M - bGap) / 2, bH = 1.85;
bens.forEach(([lbl, nb, unit, note, accent], i) => {
  const r = Math.floor(i / 2), c = i % 2;
  const cx = M + c * (bW + bGap), cy = bY + r * (bH + 0.35);
  s.addShape(pres.shapes.LINE, { x: cx, y: cy, w: bW, h: 0, line: { color: accent ? C.ikb : C.ink, width: accent ? 3 : 2 } });
  s.addText(lbl, { x: cx, y: cy + 0.12, w: bW, h: 0.3, fontFace: F.mono, fontSize: 11, color: C.grey3, charSpacing: 2 });
  s.addText([{ text: nb }, { text: unit ? "  " + unit : "", options: { fontSize: 16, color: C.grey3 } }],
    { x: cx, y: cy + 0.45, w: bW, h: 0.85, fontFace: F.sans, fontSize: 44, bold: true, color: C.ink });
  s.addText(note, { x: cx, y: cy + 1.35, w: bW, h: 0.4, fontFace: F.sans, fontSize: 13, color: C.grey3 });
});

// ============ P8 · 使用的模型 ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "MODELS · 模型与技术栈", "08 / 12");
kicker(s, "MODEL STACK", M, 1.4);
title(s, "使用的模型", M, 1.8, 9, 40);
const models = [
  ["向量化 · 检索", "Doubao Ark Embeddings", "OpenAI 兼容 /embeddings，固定维度 + ErrDimMismatch 守护 pgvector", false],
  ["向量库", "PostgreSQL · pgvector", "余弦检索，top-k 召回教材片段", false],
  ["命题生成 LLM", "DeepSeek v4 · Qwen3 (DashScope) · Doubao-seed-2.0", "平台多模型路由，温度 0.3 保稳定", true],
  ["目录 OCR", "视觉模型", "教材目录图 → 结构化「课程→章→节」知识点树", false],
  ["命题知识", "SKILL.md 命题技能", "《妇产科国考命题专家》A1–A4 规范 + 伪 A2 识别，注入式 few-shot", false],
];
let my = 3.0;
const mRowH = 0.84;
models.forEach(([use, model, note, accent]) => {
  hairline(s, M, my, W - 2 * M, accent ? C.ikb : C.borderSubtle);
  if (accent) s.addShape(pres.shapes.LINE, { x: M, y: my, w: W - 2 * M, h: 0, line: { color: C.ikb, width: 2 } });
  s.addText(use, { x: M, y: my + 0.16, w: 2.9, h: 0.5, fontFace: F.mono, fontSize: 11.5, color: accent ? C.ikb : C.grey3, charSpacing: 1.5 });
  s.addText(model, { x: M + 3.0, y: my + 0.12, w: 9.0, h: 0.35, fontFace: F.sans, fontSize: 15.5, bold: true, color: C.ink });
  s.addText(note, { x: M + 3.0, y: my + 0.46, w: 9.0, h: 0.3, fontFace: F.sans, fontSize: 12, color: C.grey3 });
  my += mRowH;
});

// ============ P9 · 消融对比（原生图表） ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "ABLATION · 消融对比", "09 / 12");
kicker(s, "CONSTRAINTS PAY OFF", M, 1.4);
title(s, "约束机制的增益", M, 1.8, 9, 40);
s.addText("指标 = 题目直接可用率（示意值，待 eval 跑分填实）",
  { x: M, y: 2.7, w: 9, h: 0.3, fontFace: F.sans, fontSize: 13, color: C.grey3 });
s.addChart(pres.charts.BAR, [{
  name: "直接可用率", labels: ["裸 prompt", "+ RAG 检索", "+ 种子锚定", "+ 技能 + 校验"], values: [28, 52, 68, 91],
}], {
  x: M, y: 3.2, w: W - 2 * M, h: 3.2, barDir: "bar",
  chartColors: [C.ikb], chartColorsOpacity: [100],
  showValue: true, dataLabelPosition: "outEnd", dataLabelColor: C.ink, dataLabelFontFace: F.sans, dataLabelFontSize: 14, dataLabelFontBold: true,
  catAxisLabelColor: C.ink, catAxisLabelFontFace: F.sans, catAxisLabelFontSize: 13,
  valAxisHidden: true, valGridLine: { style: "none" }, catAxisLineShow: false, valAxisMaxVal: 100, valAxisMinVal: 0,
  showLegend: false, showTitle: false, barGapWidthPct: 60,
});
s.addText("※ 示意指标，非真实跑分；评测口径：伪 A2 率 / 区分度 / pattern 一致率 / 溯源命中率",
  { x: M, y: 6.6, w: W - 2 * M, h: 0.3, fontFace: F.sans, fontSize: 11, color: C.grey3 });

// ============ P10 · 横向模型对比 ============
s = pres.addSlide();
s.background = { color: C.grey1 };
chrome(s, "MODELS HEAD-TO-HEAD · 横向对比", "10 / 12");
kicker(s, "SAME SKILL · SAME SEED", M, 1.4);
title(s, "同 SKILL、同种子，跑同批 A2", M, 1.8, 11, 36);
const cmp = [
  ["DeepSeek v4-pro", "复杂推理", "适用：复杂 A3 / A4 多阶段临床推理", ["临床真实感 — 待评测", "区分度 — 待评测"], false],
  ["Qwen3-max", "中文语料", "适用：中文医学语料强、术语准确", ["临床真实感 — 待评测", "区分度 — 待评测"], false],
  ["Doubao-seed-2.0", "大批量", "适用：大批量 A1 / A2，速度快、成本低", ["速度 / 成本 — ★ 优", "伪 A2 率 — 待评测"], true],
];
const cY = 3.0, cGap = 0.3, ccW = (W - 2 * M - 2 * cGap) / 3, ccH = 3.4;
cmp.forEach(([name, big, use, metrics, accent], i) => {
  const cx = M + i * (ccW + cGap);
  s.addShape(pres.shapes.RECTANGLE, { x: cx, y: cY, w: ccW, h: ccH, fill: { color: accent ? C.ikb : C.paper }, line: { type: "none" } });
  s.addShape(pres.shapes.LINE, { x: cx, y: cY, w: ccW, h: 0, line: { color: accent ? C.ikb : C.ink, width: 3 } });
  s.addText(name, { x: cx + 0.3, y: cY + 0.28, w: ccW - 0.6, h: 0.3, fontFace: F.mono, fontSize: 11, color: accent ? "C7D2F2" : C.grey3, charSpacing: 1 });
  s.addText(big, { x: cx + 0.3, y: cY + 0.7, w: ccW - 0.6, h: 0.5, fontFace: F.sans, fontSize: 24, bold: true, color: accent ? C.white : C.ink });
  s.addText(use, { x: cx + 0.3, y: cY + 1.4, w: ccW - 0.6, h: 0.7, fontFace: F.sans, fontSize: 12.5, color: accent ? C.white : C.grey3, transparency: accent ? 8 : 0, lineSpacingMultiple: 1.25 });
  s.addText(metrics.map((m, k) => ({ text: m, options: { breakLine: true, fontSize: 12, color: accent ? C.white : C.grey3, paraSpaceAfter: 6 } })),
    { x: cx + 0.3, y: cY + ccH - 1.05, w: ccW - 0.6, h: 0.9, fontFace: F.sans });
});
s.addText("评测维度：伪 A2 率 · 区分度 · pattern 一致率 · 溯源命中率 · 单题时延 / 成本",
  { x: M, y: 6.7, w: W - 2 * M, h: 0.3, fontFace: F.sans, fontSize: 11, color: C.grey3 });

// ============ P11 · 成效与展望 ============
s = pres.addSlide();
s.background = { color: C.paper };
chrome(s, "IMPACT & ROADMAP · 成效与展望", "11 / 12");
// 左：成效
kicker(s, "IMPACT · 成效", M, 1.5);
title(s, "已经做到", M, 1.95, 5, 34);
s.addShape(pres.shapes.RECTANGLE, { x: M, y: 3.2, w: 5.5, h: 2.6, fill: { color: C.ink }, line: { type: "none" } });
s.addText("一份妇产科标准卷", { x: M + 0.35, y: 3.55, w: 4.8, h: 0.5, fontFace: F.sans, fontSize: 20, bold: true, color: C.white });
s.addText("从建库 → 种子 → 命题 → 组卷 → 发布，A1–A4 配比，对话内闭环完成。",
  { x: M + 0.35, y: 4.2, w: 4.8, h: 1.3, fontFace: F.sans, fontSize: 14, color: C.white, transparency: 12, lineSpacingMultiple: 1.35 });
// 右：展望
const rX = 7.1, rW = W - M - rX;
s.addText("ROADMAP · 展望", { x: rX, y: 1.5, w: rW, h: 0.3, fontFace: F.mono, fontSize: 11, bold: true, color: C.grey3, charSpacing: 3 });
const road = [
  ["当前局限", "组卷为顺序抽样，非智能编排", false],
  ["智能分层抽样", "按知识点 / 难度自动配平", false],
  ["跨学科 pattern 扩展", "proof_based / case_study", false],
  ["多模型择优 + 专家盲评回流", "回流强化命题技能", true],
];
let ry = 2.0;
road.forEach(([t, d, accent]) => {
  hairline(s, rX, ry, rW, accent ? C.ikb : C.borderSubtle);
  if (accent) s.addShape(pres.shapes.LINE, { x: rX, y: ry, w: rW, h: 0, line: { color: C.ikb, width: 2 } });
  s.addText(t, { x: rX, y: ry + 0.16, w: rW, h: 0.4, fontFace: F.sans, fontSize: 16, bold: true, color: accent ? C.ikb : C.ink });
  s.addText(d, { x: rX, y: ry + 0.58, w: rW, h: 0.35, fontFace: F.sans, fontSize: 12.5, color: C.grey3 });
  ry += 1.18;
});

// ============ P12 · 收尾 ============
s = pres.addSlide();
s.background = { color: C.ikb };
chrome(s, "12 / 12", "CLOSING", true);
s.addText("MANIFESTO", { x: M, y: 2.0, w: 6, h: 0.3, fontFace: F.mono, fontSize: 11, color: C.white, charSpacing: 3, transparency: 22 });
s.addText([{ text: "建库一次，", options: { breakLine: true } }, { text: "命题", options: {} }, { text: "无限", options: { italic: true } }, { text: "。", options: {} }],
  { x: M, y: 2.5, w: 6.2, h: 2.0, fontFace: F.sans, fontSize: 50, color: C.white, lineSpacingMultiple: 1.0 });
s.addText("知识库 → 种子 → 命题 → 组卷 → 发布，一个对话内闭环。",
  { x: M, y: 4.9, w: 5.8, h: 0.8, fontFace: F.sans, fontSize: 14, color: C.white, transparency: 14, lineSpacingMultiple: 1.3 });
// 右侧 takeaways
const tX = 7.4, tW = W - M - tX;
s.addText("TAKEAWAYS · 03 RULES", { x: tX, y: 1.4, w: tW, h: 0.3, fontFace: F.mono, fontSize: 10.5, color: C.white, charSpacing: 2, transparency: 22 });
const takes = [
  ["01", "稳定", "种子锚定 + 技能驱动 + pattern 强校验，A1–A4 稳定可控。", false],
  ["02", "可信", "教材溯源 + 伪 A2 质检，组卷即发布给考生。", false],
  ["03", "闭环", "OIDC SSO 写回 exam，从教材到考生一气呵成。", true],
];
let ty = 2.0;
takes.forEach(([nb, t, d, accent]) => {
  s.addShape(pres.shapes.LINE, { x: tX, y: ty, w: tW, h: 0, line: { color: C.white, width: 1, transparency: accent ? 0 : 65 } });
  s.addText(nb, { x: tX, y: ty + 0.2, w: 1.0, h: 0.7, fontFace: F.sans, fontSize: 30, color: C.white, transparency: accent ? 0 : 18 });
  s.addText(t, { x: tX + 1.1, y: ty + 0.18, w: tW - 1.1, h: 0.4, fontFace: F.sans, fontSize: 19, bold: true, color: C.white });
  s.addText(d, { x: tX + 1.1, y: ty + 0.62, w: tW - 1.1, h: 0.6, fontFace: F.sans, fontSize: 12.5, color: C.white, transparency: 14, lineSpacingMultiple: 1.25 });
  ty += 1.5;
});

pres.writeFile({ fileName: "智能组卷智能体.pptx" }).then((f) => console.log("written:", f));
