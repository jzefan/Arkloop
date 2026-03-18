package artifactguidelines

var moduleSections = map[string][]string{
	"interactive": {sectionWidgetContract, sectionWidgetTheme, sectionColorPalette, sectionUIComponents},
	"chart":       {sectionWidgetContract, sectionWidgetTheme, sectionColorPalette, sectionUIComponents, sectionCharts},
	"diagram":     {sectionWidgetContract, sectionWidgetTheme, sectionColorPalette, sectionSVGSetup, sectionDiagrams},
	"art":         {sectionSVGSetup, sectionArt},
}

const guidelineCore = `# Artifact Design Guidelines

## Streaming architecture

Structure code so useful content appears early during streaming:
1. <style> block first
2. HTML content next
3. <script> block last

Rules:
- No HTML comments
- No inline event handlers; attach listeners in the final <script>
- Two font weights only: 400 and 500
- Sentence case only, never Title Case or ALL CAPS
- No gradients, shadows, blur, glow, or glass effects
- No localStorage or sessionStorage
- No position: fixed

## Theme integration

Arkloop widgets inherit host CSS variables through the iframe runtime.
Never hardcode colors directly in the final widget.
The host exposes Arkloop variables such as:
- --c-bg-page
- --c-bg-sub
- --c-text-primary
- --c-text-secondary
- --c-text-tertiary
- --c-border
- --c-border-subtle
- --c-border-mid

## Interaction bridge

Preferred bridge:
` + "`" + `window.arkloop.sendPrompt("user selected option A")` + "`" + `

Backward-compatible bridge:
` + "`" + `window.dispatchEvent(new CustomEvent('arkloop:send-prompt', { detail: text }))` + "`" + `

## Layout baseline

- Max width: 100% of container
- Outer widget root: display: block; width: 100%; background: transparent
- Component radius: 8px
- Card radius: 12px
- Border: 0.5px solid var(--c-border-subtle)
- Keep the DOM self-contained; do not rely on host page selectors`

const sectionWidgetContract = `## Widget rendering contract

Use widgets for:
- Data charts
- Flow diagrams, architecture diagrams, explanatory diagrams
- Interactive demos with sliders, toggles, or clickable nodes
- Comparison cards and dashboards

Current platform contract:
- Do NOT emit literal <widget> wrapper tags
- When using show_widget, put the snake_case identifier in the tool's title field
- Put only the inner HTML or SVG fragment in widget_code
- title must be unique per widget and suitable for client ids and filenames

Example mapping:
- Spec form: <widget title="revenue_q4">...</widget>
- Actual tool call: title="revenue_q4", widget_code="..."

Multiple widgets:
- Use separate show_widget calls
- Add normal explanatory text between widgets
- Never emit two consecutive widgets without any surrounding explanation

Strict code order:
1. <style>
2. HTML
3. <script>

Recommended shell:
- Add a single .widget-root wrapper
- Keep the outer background transparent
- Avoid viewport-sized layouts`

const sectionWidgetTheme = `## Widget theme rules

At the top of the <style> block, define local aliases from Arkloop vars:

:root {
  --color-text-primary: var(--c-text-primary);
  --color-text-secondary: var(--c-text-secondary);
  --color-background-primary: var(--c-bg-sub);
  --color-background-secondary: var(--c-bg-page);
  --color-border-tertiary: var(--c-border-subtle);
  --color-border-secondary: var(--c-border-mid);
}

Then use only these widget-facing aliases in the rest of the widget.
Do not hardcode hex colors for UI chrome.

Semantic aliases should be declared locally from the palette:
- --color-text-info / --color-background-info
- --color-text-success / --color-background-success
- --color-text-warning / --color-background-warning
- --color-text-danger / --color-background-danger

Typography:
- Main labels: 14px
- Secondary info: 12px
- Minimum size: 11px
- Body and labels use font-weight 400 or 500 only
- Line height: 1.7

Animation:
- Optional but recommended for entry
- Respect prefers-reduced-motion

Recommended entry animation:
@keyframes fadeUp {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}
.widget-root { animation: fadeUp .25s ease; }
@media (prefers-reduced-motion: reduce) {
  .widget-root { animation: none; }
}`

const sectionColorPalette = `## Color palette

Use these nine ramps. Each ramp has seven stops from light to dark.

Purple: #EEEDFE #CECBF6 #AFA9EC #7F77DD #534AB7 #3C3489 #26215C
Teal: #E1F5EE #9FE1CB #5DCAA5 #1D9E75 #0F6E56 #085041 #04342C
Coral: #FAECE7 #F5C4B3 #F0997B #D85A30 #993C1D #712B13 #4A1B0C
Blue: #E6F1FB #B5D4F4 #85B7EB #378ADD #185FA5 #0C447C #042C53
Amber: #FAEEDA #FAC775 #EF9F27 #BA7517 #854F0B #633806 #412402
Green: #EAF3DE #C0DD97 #97C459 #639922 #3B6D11 #27500A #173404
Red: #FCEBEB #F7C1C1 #F09595 #E24B4A #A32D2D #791F1F #501313
Pink: #FBEAF0 #F4C0D1 #ED93B1 #D4537E #993556 #72243E #4B1528
Gray: #F1EFE8 #D3D1C7 #B4B2A9 #888780 #5F5E5A #444441 #2C2C2A

Usage rules:
- Use the lightest stop for subtle fills
- Use dark stops for text emphasis
- Use medium-dark stops for borders and key strokes
- Color expresses meaning, not sequence
- Do not assign arbitrary step order by hue
- Keep to 2-3 ramps per widget maximum`

const sectionUIComponents = `## UI components

Cards:
- Background: var(--color-background-primary)
- Border: 0.5px solid var(--color-border-tertiary)
- Border radius: 12px
- Padding: 16px

Buttons and chips:
- Border radius: 8px
- Border: 0.5px solid var(--color-border-tertiary)
- Use sentence case labels

Widget root:
- display: block
- width: 100%
- background: transparent

Do not use:
- box-shadow
- filter: blur(...)
- backdrop-filter
- decorative glow
- fixed positioning`

const sectionCharts = `## Charts (Chart.js)

Load the UMD build from the approved CDN:
<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.1/chart.umd.js"></script>

Canvas setup:
<div style="position:relative; height:300px;">
  <canvas id="myChart"></canvas>
</div>

Required options:
{
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: false } }
}

Rules:
- Always wrap canvas in a container with explicit height
- Build legends in HTML, not with the default Chart.js legend
- Use locale-aware number formatting for money and grouped numbers
- Never display raw floating point precision noise
- Use Math.round() for integers
- Use value.toFixed(2) for two-decimal display
- Use Intl.NumberFormat for currency and thousands separators

Suggested defaults:
- Grid lines: var(--color-border-tertiary), 0.5px
- Tick text: var(--color-text-secondary), 11px
- Card background: var(--color-background-primary)

Chart selection:
- Prefer bars or lines over pie when categories exceed five
- Avoid visual decoration that competes with the data`

const sectionSVGSetup = `## SVG setup

SVG contract:
- Use width="100%"
- Use a viewBox with fixed width 680
- All coordinates must stay non-negative
- Do not rotate text
- All connector paths must use fill="none"

Example:
<svg width="100%" viewBox="0 0 680 360">

Arrow marker pattern:
<defs>
  <marker id="arrowhead" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
    <path d="M0,0 L8,3 L0,6" fill="none" stroke="context-stroke" stroke-width="1"/>
  </marker>
</defs>

Text sizing guidance:
- 14px normal text: about 7.5px per character
- 12px normal text: about 6.5px per character
- Add at least 24px horizontal padding when sizing nodes from labels`

const sectionDiagrams = `## Diagrams

Pick the layout by intent:
- "how does it work" -> flowchart
- "architecture" or "structure" -> structural diagram
- "explain" or "illustrate" -> explanatory diagram

Flowchart rules:
- One direction only: top-to-bottom or left-to-right
- Decision nodes need explicit labeled exits
- Keep labels short

Structural diagram rules:
- Parent centered over children
- Keep connectors outside node interiors
- Group related regions with subtle background panels, not shadows

Diagram limits:
- Max 5 words per node label
- Max 4 boxes per horizontal tier
- If a diagram needs more than 20 nodes, split it into multiple widgets`

const sectionArt = `## Visual art

Use SVG for illustrations and lightweight visual compositions.

Rules:
- Transparent outer background
- No filters, blur, glow, or drop shadow
- Prefer line, shape, rhythm, and spacing over decoration
- Keep text minimal
- Use the palette sparingly`

func getGuidelines(modules []string) string {
	seen := map[string]struct{}{}
	sections := []string{guidelineCore}
	for _, module := range modules {
		for _, section := range moduleSections[module] {
			if _, ok := seen[section]; ok {
				continue
			}
			seen[section] = struct{}{}
			sections = append(sections, section)
		}
	}
	return joinSections(sections)
}

func joinSections(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += "\n\n" + parts[i]
	}
	return out
}
