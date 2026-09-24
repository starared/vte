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

/* 暗色模式自定义变量（偏暖深色） */
html.dark {
  --vte-sidebar-bg: linear-gradient(180deg, #201d1b 0%, #17150f 100%);
  --vte-sidebar-solid: #1c1a18;
  --vte-sidebar-text: #cec8bf;
  --vte-header-bg: #232120;
  --vte-card-bg: #232120;
  --vte-terminal-bg: #16140f;
}

html:not(.dark) {
  --vte-sidebar-bg: linear-gradient(180deg, #33302c 0%, #262320 100%);
  --vte-sidebar-solid: #2b2825;
  --vte-sidebar-text: #d8d2c8;
  --vte-header-bg: #fffdfa;
  --vte-card-bg: #fff;
  --vte-terminal-bg: #201d18;
}
</style>
