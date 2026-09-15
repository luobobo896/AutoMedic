// 与 styles/index.css 的 --am-* token 保持同步；
// echarts 不认 CSS 变量，图表色必须从这里读取，禁止在视图里手写 hex。
export const AM_COLORS = {
  primary: '#2b6ef0',
  success: '#2f9e44',
  warning: '#f0b03e',
  danger: '#ed5d54',
  neutral: '#98a2b3',
  axisLine: '#e5e7eb',
  splitLine: '#f0f0f0',
  labelText: '#4e5969',
}
