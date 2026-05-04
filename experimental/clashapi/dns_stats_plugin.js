/**
 * DNS Stats Plugin for Zashboard
 * 集成到现有 Zashboard UI 中的 DNS 统计模块
 * 
 * 使用方式：
 * 1. 将此文件复制到 zashboard 的 plugins 或 assets 目录
 * 2. 在 zashboard 的主 HTML 中引入：<script src="dns_stats_plugin.js"></script>
 * 3. 在合适的位置调用 initDNSStatsWidget()
 */

class DNSStatsWidget {
  constructor(containerId = 'dns-stats-widget', apiBase = '/api/dns/stats') {
    this.containerId = containerId;
    this.apiBase = apiBase;
    this.refreshInterval = 2000; // 2 秒刷新一次
    this.autoRefreshTimer = null;
  }

  /**
   * 初始化 widget
   */
  async init() {
    const container = document.getElementById(this.containerId);
    if (!container) {
      console.error(`Container #${this.containerId} not found`);
      return;
    }

    // 创建 HTML 结构
    container.innerHTML = `
      <div class="dns-stats-container">
        <div class="dns-stats-header">
          <h3>🔍 DNS 查询统计</h3>
          <div class="dns-stats-controls">
            <button class="dns-stats-btn" onclick="dnsStatsWidget.refresh()">🔄 刷新</button>
            <button class="dns-stats-btn danger" onclick="dnsStatsWidget.clear()">🗑️ 清空</button>
          </div>
        </div>

        <div class="dns-stats-grid">
          <div class="dns-stat-card">
            <div class="dns-stat-label">总查询</div>
            <div class="dns-stat-value" id="dns-total">-</div>
          </div>
          <div class="dns-stat-card success">
            <div class="dns-stat-label">成功</div>
            <div class="dns-stat-value" id="dns-success">-</div>
          </div>
          <div class="dns-stat-card failed">
            <div class="dns-stat-label">失败</div>
            <div class="dns-stat-value" id="dns-failed">-</div>
          </div>
          <div class="dns-stat-card avg">
            <div class="dns-stat-label">平均延迟</div>
            <div class="dns-stat-value" id="dns-latency">-</div>
          </div>
        </div>

        <div class="dns-stats-section">
          <h4>Top 10 域名</h4>
          <div id="dns-top-domains" class="dns-chart"></div>
        </div>

        <div class="dns-stats-section">
          <h4>Top 5 DNS 服务器</h4>
          <div id="dns-top-transports" class="dns-chart"></div>
        </div>

        <div class="dns-stats-section">
          <h4>响应码分布</h4>
          <div id="dns-rcode-stats" class="dns-badges"></div>
        </div>

        <div class="dns-stats-section">
          <h4>查询类型分布</h4>
          <div id="dns-qtype-stats" class="dns-badges"></div>
        </div>

        <div class="dns-stats-section">
          <h4>最近查询</h4>
          <div id="dns-queries-table" class="dns-table-container"></div>
        </div>
      </div>
    `;

    // 添加样式
    this.injectStyles();

    // 初始化数据
    await this.refresh();

    // 启动自动刷新
    this.startAutoRefresh();
  }

  /**
   * 注入 CSS 样式
   */
  injectStyles() {
    const style = document.createElement('style');
    style.textContent = `
      .dns-stats-container {
        padding: 20px;
        background: #f5f5f5;
        border-radius: 8px;
        margin: 20px 0;
      }

      .dns-stats-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: 20px;
        border-bottom: 2px solid #ddd;
        padding-bottom: 10px;
      }

      .dns-stats-header h3 {
        margin: 0;
        color: #333;
      }

      .dns-stats-controls {
        display: flex;
        gap: 10px;
      }

      .dns-stats-btn {
        padding: 8px 16px;
        border: none;
        border-radius: 4px;
        background: #667eea;
        color: white;
        cursor: pointer;
        font-size: 14px;
        transition: background 0.3s;
      }

      .dns-stats-btn:hover {
        background: #5568d3;
      }

      .dns-stats-btn.danger {
        background: #ef4444;
      }

      .dns-stats-btn.danger:hover {
        background: #dc2626;
      }

      .dns-stats-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
        gap: 15px;
        margin-bottom: 20px;
      }

      .dns-stat-card {
        background: white;
        padding: 15px;
        border-radius: 6px;
        box-shadow: 0 1px 3px rgba(0,0,0,0.1);
      }

      .dns-stat-card.success .dns-stat-value {
        color: #10b981;
      }

      .dns-stat-card.failed .dns-stat-value {
        color: #ef4444;
      }

      .dns-stat-card.avg .dns-stat-value {
        color: #3b82f6;
      }

      .dns-stat-label {
        font-size: 12px;
        color: #999;
        text-transform: uppercase;
        margin-bottom: 8px;
      }

      .dns-stat-value {
        font-size: 24px;
        font-weight: bold;
        color: #333;
      }

      .dns-stats-section {
        background: white;
        padding: 15px;
        border-radius: 6px;
        margin-bottom: 15px;
        box-shadow: 0 1px 3px rgba(0,0,0,0.1);
      }

      .dns-stats-section h4 {
        margin: 0 0 15px 0;
        color: #333;
        font-size: 14px;
      }

      .dns-chart {
        max-height: 250px;
        overflow-y: auto;
      }

      .dns-bar {
        display: flex;
        align-items: center;
        margin-bottom: 8px;
        gap: 10px;
      }

      .dns-bar-label {
        width: 120px;
        font-size: 12px;
        color: #666;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .dns-bar-fill {
        flex: 1;
        height: 20px;
        background: linear-gradient(90deg, #667eea, #764ba2);
        border-radius: 3px;
      }

      .dns-bar-count {
        width: 40px;
        text-align: right;
        font-weight: 600;
        color: #333;
        font-size: 12px;
      }

      .dns-badges {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
      }

      .dns-badge {
        display: inline-block;
        padding: 6px 12px;
        border-radius: 4px;
        font-size: 12px;
        font-weight: 600;
      }

      .dns-badge.success {
        background: #d1fae5;
        color: #065f46;
      }

      .dns-badge.error {
        background: #fee2e2;
        color: #991b1b;
      }

      .dns-badge.other {
        background: #fef3c7;
        color: #92400e;
      }

      .dns-table-container {
        overflow-x: auto;
      }

      .dns-table {
        width: 100%;
        border-collapse: collapse;
        font-size: 12px;
      }

      .dns-table th {
        background: #f9fafb;
        color: #666;
        font-weight: 600;
        text-align: left;
        padding: 10px;
        border-bottom: 1px solid #e5e7eb;
      }

      .dns-table td {
        padding: 10px;
        border-bottom: 1px solid #f0f0f0;
      }

      .dns-table tr:hover {
        background: #f9fafb;
      }

      .dns-empty {
        text-align: center;
        padding: 20px;
        color: #999;
      }
    `;
    document.head.appendChild(style);
  }

  /**
   * 刷新数据
   */
  async refresh() {
    try {
      const [summaryRes, queriesRes] = await Promise.all([
        fetch(`${this.apiBase}/summary`),
        fetch(`${this.apiBase}/queries?limit=30`)
      ]);

      if (!summaryRes.ok || !queriesRes.ok) {
        throw new Error('API 请求失败');
      }

      const summary = await summaryRes.json();
      const queriesData = await queriesRes.json();

      this.updateStats(summary);
      this.updateQueries(queriesData.queries || []);
    } catch (err) {
      console.error('DNS Stats Error:', err);
    }
  }

  /**
   * 更新统计数据
   */
  updateStats(summary) {
    document.getElementById('dns-total').textContent = summary.total_queries || 0;
    document.getElementById('dns-success').textContent = summary.success_queries || 0;
    document.getElementById('dns-failed').textContent = summary.failed_queries || 0;
    document.getElementById('dns-latency').textContent = (summary.avg_latency_ms || 0) + ' ms';

    // Top domains
    const topDomainsHtml = (summary.top_domains || []).map(d => `
      <div class="dns-bar">
        <div class="dns-bar-label" title="${d.domain}">${d.domain}</div>
        <div class="dns-bar-fill" style="width: ${(d.count / (summary.top_domains[0]?.count || 1)) * 100}%"></div>
        <div class="dns-bar-count">${d.count}</div>
      </div>
    `).join('');
    document.getElementById('dns-top-domains').innerHTML = topDomainsHtml || '<div class="dns-empty">暂无数据</div>';

    // Top transports
    const topTransportsHtml = (summary.top_transports || []).map(t => `
      <div class="dns-bar">
        <div class="dns-bar-label">${t.transport}</div>
        <div class="dns-bar-fill" style="width: ${(t.count / (summary.top_transports[0]?.count || 1)) * 100}%"></div>
        <div class="dns-bar-count">${t.count}</div>
      </div>
    `).join('');
    document.getElementById('dns-top-transports').innerHTML = topTransportsHtml || '<div class="dns-empty">暂无数据</div>';

    // Rcode stats
    const rcodeHtml = Object.entries(summary.rcode_stats || {}).map(([rcode, count]) => {
      const badgeClass = rcode === 'NOERROR' ? 'success' : (rcode.includes('ERR') ? 'error' : 'other');
      return `<span class="dns-badge ${badgeClass}">${rcode}: ${count}</span>`;
    }).join('');
    document.getElementById('dns-rcode-stats').innerHTML = rcodeHtml || '<div class="dns-empty">暂无数据</div>';

    // Qtype stats
    const qtypeHtml = Object.entries(summary.qtype_stats || {}).map(([qtype, count]) => {
      return `<span class="dns-badge other">${qtype}: ${count}</span>`;
    }).join('');
    document.getElementById('dns-qtype-stats').innerHTML = qtypeHtml || '<div class="dns-empty">暂无数据</div>';
  }

  /**
   * 更新查询表格
   */
  updateQueries(queries) {
    const container = document.getElementById('dns-queries-table');
    if (!queries || queries.length === 0) {
      container.innerHTML = '<div class="dns-empty">暂无查询记录</div>';
      return;
    }

    const tableHtml = `
      <table class="dns-table">
        <thead>
          <tr>
            <th>时间</th>
            <th>域名</th>
            <th>类型</th>
            <th>响应码</th>
            <th>DNS 服务器</th>
            <th>延迟 (ms)</th>
          </tr>
        </thead>
        <tbody>
          ${queries.map(q => `
            <tr>
              <td>${new Date(q.timestamp).toLocaleTimeString()}</td>
              <td>${q.domain}</td>
              <td>${q.qtype}</td>
              <td><span class="dns-badge ${q.rcode === 'NOERROR' ? 'success' : 'error'}">${q.rcode}</span></td>
              <td>${q.transport}</td>
              <td>${q.latency_ms}</td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    `;
    container.innerHTML = tableHtml;
  }

  /**
   * 启动自动刷新
   */
  startAutoRefresh() {
    this.autoRefreshTimer = setInterval(() => this.refresh(), this.refreshInterval);
  }

  /**
   * 停止自动刷新
   */
  stopAutoRefresh() {
    if (this.autoRefreshTimer) {
      clearInterval(this.autoRefreshTimer);
      this.autoRefreshTimer = null;
    }
  }

  /**
   * 清空统计
   */
  async clear() {
    if (!confirm('确定要清空所有统计数据吗？')) return;
    try {
      const res = await fetch(`${this.apiBase}/`, { method: 'DELETE' });
      if (res.ok) {
        await this.refresh();
        alert('统计数据已清空');
      }
    } catch (err) {
      console.error('Clear stats error:', err);
    }
  }
}

// 全局实例
let dnsStatsWidget = null;

/**
 * 初始化 DNS Stats Widget
 * @param {string} containerId - 容器 ID
 * @param {string} apiBase - API 基础路径
 */
function initDNSStatsWidget(containerId = 'dns-stats-widget', apiBase = '/api/dns/stats') {
  dnsStatsWidget = new DNSStatsWidget(containerId, apiBase);
  dnsStatsWidget.init();
}

// 如果页面加载完成，自动初始化
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('dns-stats-widget')) {
      initDNSStatsWidget();
    }
  });
} else {
  if (document.getElementById('dns-stats-widget')) {
    initDNSStatsWidget();
  }
}
