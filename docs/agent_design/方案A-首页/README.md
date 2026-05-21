# 方案 A · 首页（智能体目录）

z.ai 风格文字 Tab + 卡片网格（含 Dribbble 风格插画封面）

## 文件结构

- `index.html` — 入口页面
- `agents.jsx` — 智能体数据（产教融合 / 教育）+ 共享组件（PageShell、AgentIcon）
- `covers.jsx` — 五张 SVG 封面插画（评估仪表盘、能力雷达、产业学院、现场工程师、学习辅导）
- `variant-a.jsx` — 主页面组件（Tab + 卡片网格）

## 本地运行

由于使用 `<script type="text/babel" src="*.jsx">` 加载外部 JSX，浏览器需通过 HTTP(S) 加载，不能直接双击打开。

任选一种方式启动本地服务器：

    # Python 3
    python3 -m http.server 8000

    # Node (npx)
    npx serve

然后访问 `http://localhost:8000/`。

## 修改智能体

编辑 `agents.jsx` 中的 `AGENTS` 对象：

    {
      id: 'xx',          // 唯一 id，需在 covers.jsx 中提供对应封面
      name: '学习辅导',
      desc: '描述文案',
      color: '#D6336C',  // 主色
      bg: '#FFE9F0',     // 浅色背景
      enabled: true,     // false 则灰显并显示"即将上线"
    }

## 替换封面

`covers.jsx` 里每个 `Cover_xx` 是一个 React 组件，接受 `{ color, bg }` props。可直接替换为真实图片：

    function Cover_xx({ color, bg }) {
      return <img src="path/to/image.jpg" style={{width:'100%', height:'100%', objectFit:'cover'}}/>;
    }
