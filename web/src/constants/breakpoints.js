// 断点单源：CSS media query 与 JS matchMedia 都以这份常量为准。
// CSS 侧对应关系（写在 styles/index.css 顶部注释）：
//   <768px  → (max-width: 767.98px)  手机
//   <1024px → (max-width: 1023.98px) 平板
//   ≥1024px                             桌面
export const BP = { mobile: 375, tablet: 768, desktop: 1024 }

export const MQ = {
  ltTablet: `(max-width: ${BP.tablet - 0.02}px)`,
  ltDesktop: `(max-width: ${BP.desktop - 0.02}px)`,
}
