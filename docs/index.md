---
title: Agora CLI Docs
---

<div class="hero-section">
  <h1 class="hero-title">Agora CLI</h1>
  <p class="hero-subtitle">Command-line tool for Agora authentication, project management, and developer onboarding</p>
</div>

<div class="install-card">
  <h2>Install</h2>
  <div class="install-command-wrapper">
    <button class="copy-button" onclick="copyInstallCommand()" aria-label="Copy install command" title="Copy to clipboard">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
      </svg>
      <span class="copy-text">Copy</span>
    </button>
    <pre><code id="install-command">curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh</code></pre>
  </div>
</div>

<script>
function copyInstallCommand() {
  const command = document.getElementById('install-command').textContent;
  const button = document.querySelector('.copy-button');
  
  navigator.clipboard.writeText(command).then(() => {
    button.classList.add('copied');
    button.setAttribute('title', 'Copied!');
    
    setTimeout(() => {
      button.classList.remove('copied');
      button.setAttribute('title', 'Copy to clipboard');
    }, 2000);
  }).catch(err => {
    console.error('Failed to copy:', err);
  });
}
</script>

<div class="card-grid">
  <a href="install.html" class="nav-card">
    <span class="nav-card-title">Install Options</span>
    <span class="nav-card-description">macOS, Linux, Windows, and package manager installations</span>
  </a>
  
  <a href="commands.html" class="nav-card">
    <span class="nav-card-title">Commands</span>
    <span class="nav-card-description">Complete command reference with examples and options</span>
  </a>
  
  <a href="automation.html" class="nav-card">
    <span class="nav-card-title">Automation</span>
    <span class="nav-card-description">JSON output contract for scripts and CI/CD pipelines</span>
  </a>
  
  <a href="error-codes.html" class="nav-card">
    <span class="nav-card-title">Error Codes</span>
    <span class="nav-card-description">Stable exit codes and error handling patterns</span>
  </a>
</div>

<h2 class="section-title">Quick Start</h2>

```bash
agora login
agora init my-demo --template nextjs
agora project doctor --json
```

<h2 class="section-title">For AI Agents</h2>

<p>Raw Markdown documentation with predictable URLs for agents and scripts:</p>

<div class="quick-links">
  <a href="@@CLI_DOCS_MD_BASE_URL@@/index.md" class="quick-link">Index</a>
  <a href="@@CLI_DOCS_MD_BASE_URL@@/commands.md" class="quick-link">Commands</a>
  <a href="@@CLI_DOCS_MD_BASE_URL@@/automation.md" class="quick-link">Automation</a>
  <a href="@@CLI_DOCS_MD_BASE_URL@@/error-codes.md" class="quick-link">Error Codes</a>
  <a href="@@CLI_DOCS_MD_BASE_URL@@/install.md" class="quick-link">Install</a>
  <a href="@@CLI_DOCS_MD_BASE_URL@@/telemetry.md" class="quick-link">Telemetry</a>
</div>

