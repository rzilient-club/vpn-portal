const TOKEN = new URLSearchParams(window.location.search).get('token') || '';

  // ── Toast ───────────────────────────────────────────────────────────────────
  let toastTimer;
  function showToast(msg, type = 'success') {
    const t = document.getElementById('toast');
    t.textContent = msg;
    t.className = 'toast show ' + type;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => t.className = 'toast', 2500);
  }

  // ── Confirm revoke ──────────────────────────────────────────────────────────
  function confirmRevoke(pubKey, email, action) {
    document.getElementById('confirmText').textContent =
      'This will permanently remove ' + email + ' from WireGuard and delete their credentials. They will need to generate new ones.';
    document.getElementById('confirmKey').value = pubKey;
    document.getElementById('confirmForm').action = action;
    document.getElementById('confirmOverlay').classList.add('active');
  }

  function closeConfirm() {
    document.getElementById('confirmOverlay').classList.remove('active');
  }

  document.getElementById('confirmOverlay').addEventListener('click', function(e) {
    if (e.target === this) closeConfirm();
  });

  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') closeConfirm();
  });

  // ── Search / filter ─────────────────────────────────────────────────────────
  function filterPeers() {
    const q = document.getElementById('searchInput').value.toLowerCase().trim();
    const rows = document.querySelectorAll('tr[data-pubkey]');
    let visible = 0;

    rows.forEach(row => {
      const email = (row.dataset.email || '').toLowerCase();
      const name  = (row.dataset.name  || '').toLowerCase();
      const ip    = (row.dataset.ip    || '').toLowerCase();
      const match = !q || email.includes(q) || name.includes(q) || ip.includes(q);
      row.classList.toggle('hidden-row', !match);
      if (match) visible++;
    });

    const count = document.getElementById('searchCount');
    count.textContent = q ? visible + ' of ' + rows.length + ' peers' : '';
  }

  // ── Copy config ─────────────────────────────────────────────────────────────
  async function copyConfig(pubKey, btn) {
    try {
      const res = await fetch('/admin/config?token=' + TOKEN + '&key=' + encodeURIComponent(pubKey));
      if (!res.ok) throw new Error('Not found');
      const config = await res.text();
      await navigator.clipboard.writeText(config);
      btn.textContent = '✓ copied';
      btn.classList.add('copied');
      showToast('Config copied to clipboard');
      setTimeout(() => {
        btn.textContent = '⎘ config';
        btn.classList.remove('copied');
      }, 2000);
    } catch(e) {
      showToast('Failed to copy config', 'error');
    }
  }

  // ── Export CSV ──────────────────────────────────────────────────────────────
  function exportCSV() {
    const rows = document.querySelectorAll('tr[data-pubkey]');
    const lines = ['Email,Name,VPN IP,Created,Status,RX,TX'];

    rows.forEach(row => {
      const rx = row.querySelector('[data-stat="rx"]')?.textContent || '—';
      const tx = row.querySelector('[data-stat="tx"]')?.textContent || '—';
      lines.push([
        row.dataset.email,
        row.dataset.name,
        row.dataset.ip,
        row.dataset.created,
        row.dataset.blocked === 'true' ? 'blocked' : 'active',
        rx,
        tx,
      ].map(v => '"' + (v || '').replace(/"/g, '""') + '"').join(','));
    });

    const blob = new Blob([lines.join('\n')], { type: 'text/csv' });
    const url  = URL.createObjectURL(blob);
    const a    = document.createElement('a');
    a.href     = url;
    a.download = 'rzilient-vpn-peers-' + new Date().toISOString().slice(0,10) + '.csv';
    a.click();
    URL.revokeObjectURL(url);
    showToast('CSV exported');
  }

  // ── Stats polling ───────────────────────────────────────────────────────────
  let lastHandshakes = {};

  function timeAgo(ts) {
    if (!ts || ts === 0) return 'never';
    const diff = Math.floor(Date.now() / 1000) - ts;
    if (diff < 60)    return diff + 's ago';
    if (diff < 3600)  return Math.floor(diff / 60) + 'm ago';
    if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
    return Math.floor(diff / 86400) + 'd ago';
  }

  // Tick last-seen every second using cached timestamps
  function tickLastSeen() {
    document.querySelectorAll('tr[data-pubkey]').forEach(row => {
      const key = row.dataset.pubkey;
      const ts  = lastHandshakes[key];
      const seenEl = row.querySelector('[data-stat="lastseen"]');
      if (seenEl && ts) seenEl.textContent = timeAgo(ts);
    });
  }

  async function fetchStats() {
    const dot   = document.getElementById('refreshDot');
    const label = document.getElementById('refreshLabel');

    dot.classList.add('spinning');
    label.textContent = 'updating...';

    try {
      const res = await fetch('/admin/stats?token=' + TOKEN);
      if (!res.ok) throw new Error('Failed');
      const stats = await res.json();

      document.querySelectorAll('tr[data-pubkey]').forEach(row => {
        const key = row.dataset.pubkey;
        const s   = stats[key];
        const rxEl    = row.querySelector('[data-stat="rx"]');
        const txEl    = row.querySelector('[data-stat="tx"]');
        const dotEl   = row.querySelector('[data-stat="dot"]');
        const seenEl  = row.querySelector('[data-stat="lastseen"]');

        if (s) {
          lastHandshakes[key] = s.last_handshake;
          if (rxEl)  rxEl.textContent  = s.rx_formatted;
          if (txEl)  txEl.textContent  = s.tx_formatted;
          if (seenEl) seenEl.textContent = timeAgo(s.last_handshake);
          if (dotEl) {
            dotEl.classList.toggle('online',  s.online);
            dotEl.classList.toggle('offline', !s.online);
          }
        } else {
          if (rxEl)  rxEl.textContent  = '—';
          if (txEl)  txEl.textContent  = '—';
          if (seenEl) seenEl.textContent = 'never';
          if (dotEl) {
            dotEl.classList.remove('online');
            dotEl.classList.add('offline');
          }
        }
      });

      label.textContent = 'updated ' + new Date().toLocaleTimeString();
    } catch(e) {
      label.textContent = 'update failed';
    } finally {
      dot.classList.remove('spinning');
    }
  }

  // ── Total traffic summary ────────────────────────────────────────────────────
  function updateTotals(stats) {
    let totalRx = 0, totalTx = 0;
    Object.values(stats).forEach(s => {
      totalRx += s.rx_bytes || 0;
      totalTx += s.tx_bytes || 0;
    });
    document.getElementById('totalRx').textContent = formatBytes(totalRx);
    document.getElementById('totalTx').textContent = formatBytes(totalTx);
  }

  function formatBytes(b) {
    if (!b || b === 0) return '0 B';
    const units = ['B','KB','MB','GB','TB'];
    let i = 0;
    while (b >= 1024 && i < units.length - 1) { b /= 1024; i++; }
    return b.toFixed(1) + ' ' + units[i];
  }

  // ── Sort by column ────────────────────────────────────────────────────────
  let sortState = { col: null, dir: 1 };

  function sortTable(col) {
    const tbody = document.querySelector('tbody');
    const rows  = Array.from(tbody.querySelectorAll('tr[data-pubkey]'));

    // Toggle direction if same column
    if (sortState.col === col) {
      sortState.dir *= -1;
    } else {
      sortState.col = col;
      sortState.dir = 1;
    }

    // Update header indicators
    document.querySelectorAll('thead th.sortable').forEach(th => {
      th.classList.remove('sort-asc', 'sort-desc');
      if (th.dataset.col === col) {
        th.classList.add(sortState.dir === 1 ? 'sort-asc' : 'sort-desc');
      }
    });

    rows.sort((a, b) => {
      let aVal, bVal;

      switch(col) {
        case 'email':
        case 'name':
        case 'ip':
        case 'created':
        case 'blocked':
          aVal = (a.dataset[col] || '').toLowerCase();
          bVal = (b.dataset[col] || '').toLowerCase();
          return aVal.localeCompare(bVal) * sortState.dir;

        case 'rx':
          aVal = lastRxBytes[a.dataset.pubkey] || 0;
          bVal = lastRxBytes[b.dataset.pubkey] || 0;
          return (aVal - bVal) * sortState.dir;

        case 'lastseen':
          aVal = lastHandshakes[a.dataset.pubkey] || 0;
          bVal = lastHandshakes[b.dataset.pubkey] || 0;
          return (aVal - bVal) * sortState.dir;

        default:
          return 0;
      }
    });

    rows.forEach(row => tbody.appendChild(row));
  }

  // Track raw bytes for sorting
  let lastRxBytes = {};

  // Patch fetchStats to update totals and raw bytes
  const _origFetchStats = fetchStats;
  fetchStats = async function() {
    const dot   = document.getElementById('refreshDot');
    const label = document.getElementById('refreshLabel');
    dot.classList.add('spinning');
    label.textContent = 'updating...';

    try {
      const res = await fetch('/admin/stats?token=' + TOKEN);
      if (!res.ok) throw new Error('Failed');
      const stats = await res.json();

      document.querySelectorAll('tr[data-pubkey]').forEach(row => {
        const key = row.dataset.pubkey;
        const s   = stats[key];
        const rxEl   = row.querySelector('[data-stat="rx"]');
        const txEl   = row.querySelector('[data-stat="tx"]');
        const dotEl  = row.querySelector('[data-stat="dot"]');
        const seenEl = row.querySelector('[data-stat="lastseen"]');

        if (s) {
          lastHandshakes[key] = s.last_handshake;
          lastRxBytes[key]    = s.rx_bytes;
          if (rxEl)  rxEl.textContent  = s.rx_formatted;
          if (txEl)  txEl.textContent  = s.tx_formatted;
          if (seenEl) seenEl.textContent = timeAgo(s.last_handshake);
          if (dotEl) {
            dotEl.classList.toggle('online',  s.online);
            dotEl.classList.toggle('offline', !s.online);
          }
        } else {
          if (rxEl)  rxEl.textContent  = '—';
          if (txEl)  txEl.textContent  = '—';
          if (seenEl) seenEl.textContent = 'never';
          if (dotEl) {
            dotEl.classList.remove('online');
            dotEl.classList.add('offline');
          }
        }
      });

      updateTotals(stats);
      label.textContent = 'updated ' + new Date().toLocaleTimeString();

      // Re-apply sort if active
      if (sortState.col) sortTable(sortState.col);

    } catch(e) {
      label.textContent = 'update failed';
    } finally {
      dot.classList.remove('spinning');
    }
  };

  // Initial fetch + poll every 30s + tick every second
  fetchStats();
  setInterval(fetchStats, 30000);
  setInterval(tickLastSeen, 1000);