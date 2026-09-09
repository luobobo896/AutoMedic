// 与 styles/index.css 的 --am-* token 保持同步；
// echarts 不认 CSS 变量，图表色必须从这里读取，禁止在视图里手写 hex。
export const AM_COLORS = {
  primary: '#6aa1ff',
  success: '#4cc38a',
  warning: '#e5a54b',
  danger: '#f2707f',
  neutral: '#5c6478',
  axisLine: '#2e2e33',
  splitLine: '#232328',
  labelText: '#9a9aa3',
}
