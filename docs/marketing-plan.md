# ghdeb 宣传计划（2026-08）

> 现状基线（2026-08-03）：创建 2 天，**5 ⭐ / 1 🍴 / 0 订阅**
> 定位：面向 Linux 开发者的 .deb 一键安装工具（"GitHub 版 apt"）

## 一、目标（一个月，8/3 → 9/3）

| 指标 | 保守目标 | 进取目标(推荐) | 说明 |
|------|---------|---------------|------|
| ⭐ Star | 30 | **80~100** | 从 5 起，需靠内容分发 + 社区 |
| 🍴 Fork | 5 | **15~20** | 跟随 star 自然增长 |
| 每周增速 | ~7/周 | 需内容矩阵支撑 |

**达成关键**：这个项目是 **niche 刚需工具**（Linux 用户装 gh/bat/fd 都很痛），痛点真实、故事性强，适合病毒式传播。但**纯 GitHub 自然流量极低**，必须靠**内容矩阵 + 社区渠道**外推。

## 二、核心叙事（一句话钩子）

> **"一个命令装齐 GitHub 上的 .deb，再也不用手动 wget + dpkg。"**

三个传播卖点：
1. **对比痛点**：bat/fd/ripgrep/gh 都要手动下载安装 → ghdeb 一条命令
2. **零配置 + 多架构**：开箱即用，支持 amd64/arm64/armhf/loong64/riscv64
3. **安全**：无 PPA、无第三方源，直接来自官方 GitHub Release，校验 .deb

## 三、渠道矩阵（按优先级）

### 1. 内容分发（最高杠杆，占 60% 精力）
- **技术博客 2-3 篇**（知乎/掘金/CSDN/个人博客，先发平台再引流 GitHub）：
  - 篇1：《我写了个 "GitHub 版 apt"，一条命令装完所有 .deb》
  - 篇2：《为什么 Debian 用户装软件这么痛苦，以及我的解法》
  - 篇3：《5 种架构交叉编译 + 自动识别 .deb，ghdeb 设计笔记》
- **1 分钟演示 GIF**：终端录屏 install → update → list，放 README 顶部
- **中文内容发布到**：微信公众号（可用 gzh-design 技能排）、知乎、掘金、CSDN、V2EX、Linux 中国
- **英文内容发布到**：dev.to、Hacker News（Show HN）、Reddit r/linux r/debian、lobste.rs

### 2. 社区渠道（占 30%）
- **Show HN** 发帖（最佳冷启动渠道，一条标题党帖可能带来几百 star）
- **Reddit**：r/linux、r/debian、r/selfhosted、r/commandline
- **中文社区**：V2EX、Linux 中国、少数派、思否
- **主题讨论串**：在 "how to install .deb" 类帖子下自然植入（别硬广）

### 3. GitHub 内部（占 10%）
- 完善 README：顶部 GIF + 徽章 + 明确 star 号召（"Star this repo"）
- 发布 v0.7.x Release 时发 **Release 公告**
- 加 **GitHub Topics**（.deb、github-releases、linux、debian、apt）
- 响应 issue/PR 建立活跃氛围

## 四、执行日历（4 周）

**第 1 周 · 地基**
- [ ] 美化 README（GIF 演示 + 徽章 + star 号召）
- [ ] 加 GitHub Topics
- [ ] 发布 Show HN + Reddit 首发帖
- [ ] 篇1 技术博客上线（含安装演示）

**第 2 周 · 中文引爆**
- [ ] 微信公众号推文（gzh-design 排版）
- [ ] 知乎/掘金/CSDN 三平台同步
- [ ] V2EX 发帖
- [ ] 篇2 上线

**第 3 周 · 英文扩散 + 社区**
- [ ] dev.to 英文教程
- [ ] Reddit 多 sub 主题回复
- [ ] 篇3（多架构设计）上线
- [ ] 收集用户反馈，出一版小改进（体现项目活跃）

**第 4 周 · 收割 + 复盘**
- [ ] 二次 Show HN 或 "1.0.0" 里程碑公告
- [ ] 汇总数据，复盘什么内容最有效
- [ ] 冲刺目标，补充长尾内容

## 五、KPI 追踪表
| 周 | 目标 Star | 累计 |
|----|----------|------|
| W1 | +15 | 20 |
| W2 | +30 | 50 |
| W3 | +25 | 75 |
| W4 | +15 | 90 |

---

# 关于微信/支付宝收款码（Donate）——可以，但建议这么做

## ✅ 结论：完全可以放，但要讲"姿势"

技术上没有任何阻碍，**国内很多知名开源项目**（如某些开发者）都在 README 放微信/支付宝收款码。你只要注意定位与合规边界：

## 合规边界（务必遵守）
1. **定位为"自愿打赏/支持开发者"**，绝不是"购买软件"。不能有"付费才给功能"的表述（那是经营性行为，需要资质）。
2. 文案用"**请我喝杯咖啡 ☕ / 支持开源**"，避免任何"捐赠解锁功能"暗示。
3. 个人收款码用于自愿打赏在现行规则下是可行的；**不要把它标成"商品/服务交易"**。

## 推荐实现方式
1. **GitHub Sponsors** 为主（国际用户 + 平台背书），README 放 Sponsors 按钮徽章。
2. **微信/支付宝收款码**放 README 底部专门的 **"Donate / 打赏支持"** 区块（很多国内用户只看这个）。
3. 收款码图片放进仓库 `docs/donate/` 目录，用 `<img>` 引入。
4. 也可加 **Buy Me a Coffee / Ko-fi** 链接（海外小额打赏友好）。

## README 建议区块（底部）
```markdown
## ☕ Support / 打赏支持

觉得 ghdeb 有用？请我喝杯咖啡～

| 微信 | 支付宝 |
|------|--------|
| ![wechat](docs/donate/wechat.png) | ![alipay](docs/donate/alipay.png) |

[![Sponsor](https://img.shields.io/github/sponsors/LeisureLinux)](https://github.com/sponsors/LeisureLinux)
```

## ⚠️ 潜在小争议（提前心理准备）
- 极少数人会吐槽"开源项目放收款码"，属于正常现象，不必在意。
- 只要你**定位是打赏而非收费**，就不会伤及项目专业性，反而让"接地气"的开发者形象加分。

---

## 六、我的建议落地顺序
1. 先定目标档位（推荐进取档 80~100 star）
2. 本周做 ①README 加 GIF + star 号召 ②加 Donate 区块 ③Show HN 首发
3. 同步产 2 篇中文博客引流
4. 边发边记录数据，W2 复盘调整

要不要我直接把 **Donate 区块** 和 **README 顶部 GIF 占位 + star 号召** 帮你写好提交？收款码图片你放 `docs/donate/` 即可。
