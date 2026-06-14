package kb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

// wizardChapter is one selectable chapter / knowledge point in the wizard.
type wizardChapter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// wizardKB is one knowledge base baked into the wizard widget, with its
// chapters preloaded so the whole 3-step flow runs client-side.
type wizardKB struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Mode     string          `json:"mode"` // "standalone" | "exam"
	Chapters []wizardChapter `json:"chapters"`
}

type wizardKBDesc struct {
	desc kbDescriptor
	name string
}

// executePaperWizard renders the guided paper-building wizard: it loads every
// ready knowledge base plus its chapters and bakes them into a single
// self-contained multi-step widget. The teacher picks KB -> chapters ->
// A1-A4 counts + difficulty entirely in the panel, then the final button
// posts one structured 【智能组卷·开始生成】 command back via sendPrompt.
func (e *Executor) executePaperWizard(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	if e.IsNotConfigured() || e.pool == nil {
		return errResult(errorNotConfigured, "kb_paper_wizard is not configured")
	}
	accountID := uuid.Nil
	if execCtx.AccountID != nil {
		accountID = *execCtx.AccountID
	}
	if accountID == uuid.Nil {
		return errResult(errorPermissionDenied, "kb_paper_wizard requires an account context")
	}
	userID := uuid.Nil
	if execCtx.UserID != nil {
		userID = *execCtx.UserID
	}

	descs, err := e.listWizardKBs(ctx, accountID, userID, listKnowledgeBasesWorkspaceRef(args))
	if err != nil {
		return errResult(errorSearchFailed, "list knowledge bases: "+err.Error())
	}
	kbs := make([]wizardKB, 0, len(descs))
	for _, d := range descs {
		kbs = append(kbs, wizardKB{
			ID:       d.desc.ID.String(),
			Name:     d.name,
			Mode:     wizardMode(d.desc.IntegrationMode),
			Chapters: e.wizardChaptersFor(ctx, d.desc, execCtx),
		})
	}

	return tools.ExecutionResult{ResultJSON: map[string]any{
		"action":          "paper_wizard",
		"knowledge_bases": kbs,
		"ui_panel": map[string]any{
			"kind":        "paper_wizard",
			"title":       "智能组卷向导",
			"widget_code": paperWizardWidget(kbs),
		},
		"instruction": "用 visualize_read_me + show_widget 展示 ui_panel.widget_code 中的组卷向导面板（不要把 HTML 当普通文本输出）。" +
			"展示后等待老师在面板里完成三步选择并点击\"开始生成\"，老师会以【智能组卷·开始生成】开头把结构化参数发回。" +
			"在此之前不要再用文字逐条向老师询问知识库/章节/题型/难度。",
	}}
}

// listWizardKBs returns the account's ready, user-kind knowledge bases with the
// descriptor fields needed to load chapters. Mirrors the visibility / ready /
// kb_kind filters used by kb_list_knowledge_bases.
func (e *Executor) listWizardKBs(ctx context.Context, accountID, userID uuid.UUID, workspaceRef string) ([]wizardKBDesc, error) {
	query := `
SELECT kb.id, kb.name, kb.workspace_ref, kb.integration_mode, kb.exam_scope_id
FROM   knowledge_bases kb
WHERE  kb.account_id = $1
  AND  (kb.visibility <> 'private' OR kb.created_by = $2)
  AND  kb.kb_kind = 'user'
  AND  EXISTS (SELECT 1 FROM kb_documents d WHERE d.kb_id = kb.id AND d.status = 'ready')`
	argsSQL := []any{accountID, userID}
	if workspaceRef != "" {
		argsSQL = append(argsSQL, workspaceRef)
		query += fmt.Sprintf("\n  AND kb.workspace_ref = $%d", len(argsSQL))
	}
	query += "\nORDER BY kb.created_at DESC, kb.id ASC"

	rows, err := e.pool.Query(ctx, query, argsSQL...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []wizardKBDesc{}
	for rows.Next() {
		var id uuid.UUID
		var name, ws, mode string
		var examScopeID *string
		if err := rows.Scan(&id, &name, &ws, &mode, &examScopeID); err != nil {
			return nil, err
		}
		out = append(out, wizardKBDesc{
			desc: kbDescriptor{ID: id, AccountID: accountID, WorkspaceRef: ws, IntegrationMode: mode, ExamScopeID: examScopeID},
			name: name,
		})
	}
	return out, rows.Err()
}

// wizardChaptersFor loads the chapters (knowledge points) for one KB, reusing
// the same standalone/linked split as kb_list_knowledge_points. Best-effort:
// on any error it returns an empty list so the KB is still selectable and the
// panel shows a "no chapters" note instead of failing the whole wizard.
func (e *Executor) wizardChaptersFor(ctx context.Context, kb kbDescriptor, execCtx tools.ExecutionContext) []wizardChapter {
	if kb.IntegrationMode == "exam" {
		return wizardChaptersFromResult(e.executeProviderListKnowledgePoints(ctx, kb, execCtx))
	}
	items, err := e.listLocalKnowledgePointItems(ctx, kb.ID)
	if err != nil {
		return nil
	}
	if len(items) == 0 {
		if inserted, derr := e.ensureFallbackKnowledgePointsFromHeadings(ctx, kb.ID); derr == nil && inserted > 0 {
			items, _ = e.listLocalKnowledgePointItems(ctx, kb.ID)
		}
	}
	out := make([]wizardChapter, 0, len(items))
	for _, kp := range items {
		out = append(out, wizardChapter{ID: kp.ID.String(), Name: kp.Name})
	}
	return out
}

// wizardChaptersFromResult adapts the provider knowledge-point listing
// (exam-linked KBs) into wizard chapters, tolerating both []map[string]any and
// []any item shapes and the common id/name field aliases.
func wizardChaptersFromResult(res tools.ExecutionResult) []wizardChapter {
	if res.ResultJSON == nil {
		return nil
	}
	rawItems := res.ResultJSON["items"]
	maps := toMapSlice(rawItems)
	out := make([]wizardChapter, 0, len(maps))
	for _, m := range maps {
		id := strings.TrimSpace(asString(m["id"]))
		if id == "" {
			continue
		}
		name := firstNonEmptyStr(asString(m["name"]), asString(m["display_name"]), asString(m["code"]), id)
		out = append(out, wizardChapter{ID: id, Name: name})
	}
	return out
}

func toMapSlice(v any) []map[string]any {
	switch items := v.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func wizardMode(mode string) string {
	if strings.TrimSpace(mode) == "exam" {
		return "exam"
	}
	return "standalone"
}

// paperWizardWidget builds the self-contained 3-step wizard widget. The KB +
// chapter data is baked in as JSON so all three steps run client-side and only
// the final "开始生成" click posts a structured command back to chat.
func paperWizardWidget(kbs []wizardKB) string {
	if len(kbs) == 0 {
		return paperWizardEmptyWidget()
	}
	data, err := json.Marshal(kbs)
	if err != nil {
		return paperWizardEmptyWidget()
	}
	return strings.Replace(paperWizardTemplate, "/*__PW_DATA__*/null", string(data), 1)
}

func paperWizardEmptyWidget() string {
	return `<style>.pw{font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#18212f;border:1px solid #d8dee8;border-radius:10px;background:#fff;padding:16px;max-width:720px}.pw h3{margin:0 0 6px;font-size:16px}.pw p{margin:0;font-size:13px;color:#667085;line-height:1.6}</style><div class="pw"><h3>智能组卷</h3><p>当前没有可用于组卷的课程资料知识库（需至少包含一个已就绪文档）。请联系管理员在管理端建设课程资料后再试。</p></div>`
}

const paperWizardTemplate = `<style>
.pw{font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#18212f;border:1px solid #d8dee8;border-radius:10px;background:#fff;padding:16px;max-width:720px}
.pw h3{margin:0 0 4px;font-size:16px}
.pw-sub{font-size:13px;color:#667085;margin:0 0 12px;line-height:1.5}
.pw-list{display:grid;gap:8px;max-height:300px;overflow:auto;margin:4px 0}
.pw-opt{display:flex;align-items:center;gap:10px;border:1px solid #edf0f5;border-radius:8px;padding:10px;background:#f8fafc;cursor:pointer}
.pw-opt input{margin:0;flex:none}
.pw-opt-t{font-size:14px}
.pw-opt-s{font-size:12px;color:#8a93a4;margin-left:auto;white-space:nowrap}
.pw-allrow{display:flex;align-items:center;gap:8px;font-size:13px;color:#1f6feb;padding:6px 2px;cursor:pointer}
.pw-empty{font-size:13px;color:#8a93a4;padding:14px;text-align:center;background:#f8fafc;border-radius:8px}
.pw-types{display:grid;gap:8px;margin:6px 0}
.pw-typerow{display:flex;align-items:center;justify-content:space-between;border:1px solid #edf0f5;border-radius:8px;padding:8px 10px;background:#f8fafc}
.pw-tlabel b{font-size:14px}
.pw-tlabel span{font-size:12px;color:#8a93a4;margin-left:8px}
.pw-stepper{display:flex;align-items:center;gap:6px}
.pw-stepper button{width:28px;height:28px;border:1px solid #d8dee8;border-radius:6px;background:#fff;cursor:pointer;font-size:16px;line-height:1}
.pw-stepper input{width:48px;text-align:center;border:1px solid #d8dee8;border-radius:6px;padding:4px;font-size:14px}
.pw-diff{display:flex;align-items:center;gap:14px;font-size:13px;margin:10px 0 4px;flex-wrap:wrap}
.pw-diff label{display:flex;align-items:center;gap:5px;cursor:pointer}
.pw-total{font-size:13px;color:#18212f;margin:8px 0}
.pw-actions{display:flex;gap:8px;margin-top:14px;flex-wrap:wrap}
.pw-btn{border:0;border-radius:8px;padding:9px 16px;background:#1f6feb;color:#fff;cursor:pointer;font-size:14px}
.pw-btn.ghost{background:#eef2f6;color:#18212f}
.pw-btn:disabled{opacity:.45;cursor:not-allowed}
</style>
<div class="pw" id="pw"></div>
<script>
(function(){
  var KBS = /*__PW_DATA__*/null;
  if(!KBS||!KBS.length){return;}
  var TYPES=[["A1","单句型最佳选择题"],["A2","病例摘要型最佳选择题"],["A3","病例组型最佳选择题"],["A4","病例串型最佳选择题"]];
  var DIFFS=[["easy","易"],["medium","中"],["hard","难"]];
  var DIFFMAP={easy:"易",medium:"中",hard:"难"};
  var s={step:1,kb:(KBS.length===1?KBS[0].id:null),sel:{},counts:{A1:0,A2:0,A3:0,A4:0},diff:"medium"};
  var root=document.getElementById('pw');
  function esc(t){var d=document.createElement('div');d.textContent=(t==null?'':String(t));return d.innerHTML;}
  function curKB(){for(var i=0;i<KBS.length;i++){if(KBS[i].id===s.kb)return KBS[i];}return null;}
  function selCount(){var n=0;for(var k in s.sel){if(s.sel[k])n++;}return n;}
  function total(){return (s.counts.A1||0)+(s.counts.A2||0)+(s.counts.A3||0)+(s.counts.A4||0);}
  function render(){
    var h='';
    if(s.step===1){
      h+='<h3>智能组卷 · 第 1/3 步：选择知识库</h3><div class="pw-sub">选择要用来出题的课程资料知识库</div><div class="pw-list">';
      for(var i=0;i<KBS.length;i++){var k=KBS[i];var modeTxt=(k.mode==='exam'?'考试题库绑定':'课程资料');h+='<label class="pw-opt"><input type="radio" name="pwkb" data-kb="'+esc(k.id)+'"'+(s.kb===k.id?' checked':'')+'><span class="pw-opt-t">'+esc(k.name)+'</span><span class="pw-opt-s">'+modeTxt+' · '+k.chapters.length+' 章节</span></label>';}
      h+='</div><div class="pw-actions"><button class="pw-btn" data-act="next"'+(s.kb?'':' disabled')+'>下一步</button></div>';
    }else if(s.step===2){
      var kb=curKB();var chs=kb?kb.chapters:[];
      h+='<h3>智能组卷 · 第 2/3 步：选择章节</h3><div class="pw-sub">知识库：'+esc(kb?kb.name:'')+'</div>';
      if(chs.length===0){
        h+='<div class="pw-empty">该知识库暂无可用章节，请返回上一步选择其他知识库。</div><div class="pw-actions"><button class="pw-btn ghost" data-act="prev">上一步</button></div>';
      }else{
        var allOn=(selCount()===chs.length);
        h+='<label class="pw-allrow"><input type="checkbox" data-act="all"'+(allOn?' checked':'')+'>全选（共 '+chs.length+' 章节）</label><div class="pw-list">';
        for(var j=0;j<chs.length;j++){var c=chs[j];h+='<label class="pw-opt"><input type="checkbox" data-kp="'+esc(c.id)+'"'+(s.sel[c.id]?' checked':'')+'><span class="pw-opt-t">'+esc(c.name)+'</span></label>';}
        h+='</div><div class="pw-actions"><button class="pw-btn ghost" data-act="prev">上一步</button><button class="pw-btn" data-act="next" id="pwNext2"'+(selCount()===0?' disabled':'')+'>下一步（已选 <span id="pwSel">'+selCount()+'</span>）</button></div>';
      }
    }else{
      h+='<h3>智能组卷 · 第 3/3 步：题型数量与难度</h3><div class="pw-sub">设置 A1-A4 各题型数量，难度整卷统一</div><div class="pw-types">';
      for(var t=0;t<TYPES.length;t++){var ty=TYPES[t][0];h+='<div class="pw-typerow"><div class="pw-tlabel"><b>'+ty+'</b><span>'+TYPES[t][1]+'</span></div><div class="pw-stepper"><button data-act="dec" data-type="'+ty+'">−</button><input type="number" min="0" max="50" data-type="'+ty+'" value="'+(s.counts[ty]||0)+'"><button data-act="inc" data-type="'+ty+'">+</button></div></div>';}
      h+='</div><div class="pw-diff">难度（整卷统一）：';
      for(var d2=0;d2<DIFFS.length;d2++){h+='<label><input type="radio" name="pwdiff" data-diff="'+DIFFS[d2][0]+'"'+(s.diff===DIFFS[d2][0]?' checked':'')+'>'+DIFFS[d2][1]+'</label>';}
      h+='</div><div class="pw-total">合计：<b id="pwTotal">'+total()+'</b> 道</div><div class="pw-actions"><button class="pw-btn ghost" data-act="prev">上一步</button><button class="pw-btn" data-act="submit" id="pwSubmit"'+(total()===0?' disabled':'')+'>开始生成</button></div>';
    }
    root.innerHTML=h;
  }
  function submit(){
    var kb=curKB();if(!kb)return;
    var chs=kb.chapters.filter(function(c){return s.sel[c.id];});
    if(chs.length===0||total()===0){return;}
    var chTxt=chs.map(function(c){return c.name+'｜kp_id='+c.id;}).join('；');
    var cmd=['【智能组卷·开始生成】',
      '知识库：'+kb.name+'｜kb_id='+kb.id+'｜模式='+kb.mode,
      '章节（'+chs.length+'）：'+chTxt,
      '题型数量：A1='+(s.counts.A1||0)+' A2='+(s.counts.A2||0)+' A3='+(s.counts.A3||0)+' A4='+(s.counts.A4||0)+'（共'+total()+'道，均为单选题 single_choice）',
      '难度：'+DIFFMAP[s.diff]+'（canonical='+s.diff+'，整卷统一）',
      '请严格按以上参数生成：A1/A2/A3/A4 四类单选题，pattern_tag 分别为 A1/A2/A3/A4；把每个题型的题量尽量均匀分布到所选章节；难度统一为'+DIFFMAP[s.diff]+'。先展示草稿预览，待我确认再保存到考试系统。'
    ].join('\n');
    if(window.sendPrompt){window.sendPrompt(cmd);}
    root.innerHTML='<div class="pw-sub">已提交组卷参数，正在为你生成题目草稿…</div>';
  }
  root.addEventListener('click',function(e){
    var el=e.target.closest('[data-act]');if(!el)return;
    var act=el.getAttribute('data-act');
    if(act==='next'){if(s.step<3){s.step++;render();}return;}
    if(act==='prev'){if(s.step>1){s.step--;render();}return;}
    if(act==='inc'||act==='dec'){var ty=el.getAttribute('data-type');var dv=(act==='inc'?1:-1);s.counts[ty]=Math.max(0,Math.min(50,(s.counts[ty]||0)+dv));render();return;}
    if(act==='submit'){submit();return;}
  });
  root.addEventListener('change',function(e){
    var t=e.target;
    if(t.name==='pwkb'){s.kb=t.getAttribute('data-kb');s.sel={};render();return;}
    if(t.name==='pwdiff'){s.diff=t.getAttribute('data-diff');return;}
    if(t.getAttribute&&t.getAttribute('data-act')==='all'){var kb=curKB();s.sel={};if(t.checked&&kb){kb.chapters.forEach(function(c){s.sel[c.id]=true;});}render();return;}
    if(t.hasAttribute&&t.hasAttribute('data-kp')){var id=t.getAttribute('data-kp');if(t.checked){s.sel[id]=true;}else{delete s.sel[id];}var sp=document.getElementById('pwSel');if(sp){sp.textContent=selCount();}var nx=document.getElementById('pwNext2');if(nx){nx.disabled=(selCount()===0);}return;}
  });
  root.addEventListener('input',function(e){
    var t=e.target;
    if(t.hasAttribute&&t.hasAttribute('data-type')&&t.type==='number'){var v=parseInt(t.value,10);if(isNaN(v)){v=0;}v=Math.max(0,Math.min(50,v));s.counts[t.getAttribute('data-type')]=v;var tot=document.getElementById('pwTotal');if(tot){tot.textContent=total();}var sb=document.getElementById('pwSubmit');if(sb){sb.disabled=(total()===0);}}
  });
  render();
})();
</script>`
