# 应用图标

这里的图标来自 [Simple Icons](https://github.com/simple-icons/simple-icons) 16.38.0（CC0-1.0），
填上了各自品牌的主色，编进可执行文件，页面上用 `data:` 地址显示，不从任何外部地址加载。

| 文件 | 品牌 | 颜色 |
| --- | --- | --- |
| claude-code.svg | Claude Code | #D97757 |
| deepseek-harness.svg | DeepSeek | #5786FE |
| redis.svg | Redis | #FF4438 |
| postgres.svg | PostgreSQL | #4169E1 |
| mariadb.svg | MariaDB | #003545 |
| portainer.svg | Portainer | #13BEF9 |
| uptime-kuma.svg | Uptime Kuma | #5CDD8B |

这些图标和名称是各自所有者的商标，在这里只用来标明是哪个应用，不表示所有者认可或赞助本项目。
没有找到可以自由分发的官方图标的应用（目前是 Codex CLI 和 3x-ui），页面上用清单里 `icon` 指定的通用图标。

新增图标的要求：单个 SVG 文件、不超过 8 KB、文件名和应用名一致；不能有脚本、外部引用（`href`）或 `foreignObject`。
