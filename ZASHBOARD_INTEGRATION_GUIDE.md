# Zashboard DNS 统计集成指南

本指南说明如何将 DNS 查询统计功能集成到 Zashboard UI 中。

## 前置条件

- 已 fork Zashboard 仓库到自己的 GitHub 账户
- 已 fork sing-box 仓库并集成了 DNS 统计功能（`feat/dns-stats-ui-experiment` 分支）

## 集成步骤

### 1. Fork Zashboard 仓库

访问 https://github.com/Zephyruso/zashboard 并 fork 到自己的账户。

### 2. 克隆你的 Zashboard fork

```bash
git clone https://github.com/YOUR_USERNAME/zashboard.git
cd zashboard
```

### 3. 添加 DNS 统计插件

#### 方式 A：复制插件文件（推荐）

从 sing-box 仓库复制插件文件到 Zashboard：

```bash
# 从 sing-box 仓库复制
cp ../sing-box/experimental/clashapi/dns_stats_plugin.js ./src/assets/

# 或手动创建文件
touch src/assets/dns_stats_plugin.js
# 然后复制 dns_stats_plugin.js 的内容
```

#### 方式 B：作为 npm 模块（高级）

在 `package.json` 中添加依赖：

```json
{
  "dependencies": {
    "dns-stats-widget": "file:../sing-box/experimental/clashapi"
  }
}
```

### 4. 修改 Zashboard 主 HTML

编辑 `index.html` 或主入口文件，在合适位置添加 DNS 统计容器：

```html
<!-- 在导航或主内容区域添加 -->
<div id="dns-stats-widget"></div>

<!-- 在 </body> 前引入插件 -->
<script src="./assets/dns_stats_plugin.js"></script>
```

### 5. 修改 Zashboard 的 Vue/React 组件（可选）

如果 Zashboard 使用框架，可以创建一个组件包装器：

**Vue 示例（`src/components/DNSStats.vue`）：**

```vue
<template>
  <div id="dns-stats-widget"></div>
</template>

<script>
export default {
  name: 'DNSStats',
  mounted() {
    // 动态加载插件
    const script = document.createElement('script');
    script.src = '/assets/dns_stats_plugin.js';
    document.body.appendChild(script);
  }
}
</script>
```

**React 示例（`src/components/DNSStats.jsx`）：**

```jsx
import { useEffect } from 'react';

export default function DNSStats() {
  useEffect(() => {
    const script = document.createElement('script');
    script.src = '/assets/dns_stats_plugin.js';
    document.body.appendChild(script);
  }, []);

  return <div id="dns-stats-widget"></div>;
}
```

### 6. 配置 API 基础路径

如果 Zashboard 的 API 路径不同，修改插件初始化：

```javascript
// 在 index.html 中
<script>
  // 自定义 API 基础路径
  window.DNS_STATS_API_BASE = '/api/dns/stats';
  
  // 或在插件加载后手动初始化
  document.addEventListener('DOMContentLoaded', () => {
    initDNSStatsWidget('dns-stats-widget', '/api/dns/stats');
  });
</script>
```

### 7. 构建和部署

```bash
# 安装依赖
npm install

# 开发模式
npm run dev

# 生产构建
npm run build

# 部署到你的服务器
# 例如复制到 /etc/momo/run/ui/
cp -r dist/* /etc/momo/run/ui/
```

## 文件结构

修改后的 Zashboard 目录结构应该如下：

```
zashboard/
├── src/
│   ├── assets/
│   │   └── dns_stats_plugin.js          # 新增：DNS 统计插件
│   ├── components/
│   │   └── DNSStats.vue (或 .jsx)       # 可选：框架组件包装器
│   └── index.html                        # 修改：添加容器和脚本引入
├── package.json
└── ...
```

## 配置 sing-box

确保 sing-box 的 Clash API 配置正确：

```json
{
  "experimental": {
    "clash_api": {
      "external_controller": "0.0.0.0:9095",
      "external_ui": "/etc/momo/run/ui",
      "external_ui_download_url": "https://gh-proxy.com/https://github.com/YOUR_USERNAME/zashboard/archive/refs/heads/main.zip",
      "external_ui_http_client": "direct-fetch",
      "secret": "",
      "default_mode": "rule"
    }
  }
}
```

## 验证集成

1. 启动 sing-box：
```bash
./sing-box run -c config.json
```

2. 访问 Zashboard：
```
http://192.168.2.1:9095
```

3. 检查 DNS 统计模块是否显示
4. 查看浏览器控制台是否有错误

## 常见问题

### Q: DNS 统计模块不显示？
A: 检查：
- 浏览器控制台是否有错误
- `/api/dns/stats/summary` 是否返回 200
- `dns_stats_plugin.js` 是否正确加载

### Q: API 返回 404？
A: 确保：
- sing-box 的 Clash API 已启用
- 路由正确：`/api/dns/stats/summary`
- 没有认证问题（检查 `secret` 配置）

### Q: 样式不匹配 Zashboard？
A: 修改 `dns_stats_plugin.js` 中的 CSS，使其与 Zashboard 的主题一致。

## 提交 PR 到上游

完成集成后，可以向 Zashboard 官方仓库提交 PR：

```bash
git add .
git commit -m "feat: add DNS stats widget integration"
git push origin main
# 然后在 GitHub 上创建 PR
```

## 相关资源

- sing-box DNS 统计 API：`experimental/clashapi/dns_stats.go`
- DNS 统计插件：`experimental/clashapi/dns_stats_plugin.js`
- Zashboard 官方仓库：https://github.com/Zephyruso/zashboard

## 支持

如有问题，请在以下位置提交 issue：
- sing-box fork：https://github.com/YOUR_USERNAME/sing-box/issues
- Zashboard fork：https://github.com/YOUR_USERNAME/zashboard/issues
