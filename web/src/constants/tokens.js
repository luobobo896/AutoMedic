// 与 styles/index.css 的 --am-* token 保持同步；
// echarts 不认 CSS 变量，图表色必须从这里读取，禁止在视图里手写 hex。
export const AM_COLORS = {
  primary: '#1f6feb',
  success: '#12a150',
  warning: '#d08700',
  danger: '#e7000b',
  neutral: '#a1a1a1',
  axisLine: '#e8e8e8',
  splitLine: '#f0f0f0',
  labelText: '#606060',
}
