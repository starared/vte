<template>
  <router-view />
</template>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: var(--el-bg-color-page);
  color: var(--el-text-color-primary);
  transition: background-color 0.3s, color 0.3s;
}

/* 暗色模式自定义变量 */
html.dark {
  --vte-sidebar-bg: #1d1e1f;
  --vte-sidebar-text: #bfcbd9;
  --vte-header-bg: #1d1e1f;
  --vte-card-bg: #1d1e1f;
  --vte-terminal-bg: #0d0d0d;
}

html:not(.dark) {
  --vte-sidebar-bg: #304156;
  --vte-sidebar-text: #bfcbd9;
  --vte-header-bg: #fff;
  --vte-card-bg: #fff;
  --vte-terminal-bg: #1e1e1e;
}
</style>
