import { ref, onMounted, onUnmounted } from 'vue'
import { MQ } from '@/constants/breakpoints'

// 响应式断点：isMobile <768px；isTablet 768–1023px；否则桌面。
// 替代各组件里的 window.innerWidth 轮询，保证与 CSS 断点同源。
// matchMedia 在 setup 阶段同步可用，初始值即时生效，避免首帧闪错布局。
export function useBreakpoint() {
  const mTablet = window.matchMedia(MQ.ltTablet)
  const mDesktop = window.matchMedia(MQ.ltDesktop)

  const isMobile = ref(mTablet.matches)
  const isTablet = ref(!mTablet.matches && mDesktop.matches)

  function update() {
    isMobile.value = mTablet.matches
    isTablet.value = !mTablet.matches && mDesktop.matches
  }

  onMounted(() => {
    mTablet.addEventListener('change', update)
    mDesktop.addEventListener('change', update)
  })
  onUnmounted(() => {
    mTablet.removeEventListener('change', update)
    mDesktop.removeEventListener('change', update)
  })

  return { isMobile, isTablet }
}
