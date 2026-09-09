#!/usr/bin/env bash
# UI token 约束检查：拦截字号/圆角漂移与视图层硬编码颜色。
# 用法：bash scripts/check-ui-style.sh（在 web/ 下通过 npm run lint:ui 调用）
set -euo pipefail
cd "$(dirname "$0")/../web/src"

fail=0

# 1. 字号必须走 --am-font-* 阶梯（12/13/14/16/18/22/26），禁止其它像素值
#    白名单：26px 统计数值在 index.css 由 var 表达，此处只查视图与全局样式里的裸值
bad_font=$(grep -rnE "font-size: ?[0-9]+(\.[0-9]+)?px" views/ layout/ styles/ 2>/dev/null || true)
if [ -n "$bad_font" ]; then
  echo "✗ 字号漂移（只允许 var(--am-font-*)）："; echo "$bad_font"; fail=1
fi

# 2. 圆角只允许 var(--am-radius-*)、50%、999px（胶囊/圆点）
bad_radius=$(grep -rnE "border-radius: ?[0-9]+px" views/ layout/ styles/ 2>/dev/null | grep -vE "999px" || true)
if [ -n "$bad_radius" ]; then
  echo "✗ 圆角漂移（只允许 var(--am-radius-*)）："; echo "$bad_radius"; fail=1
fi

# 3. 视图层禁止新增硬编码 hex 颜色（echarts 色统一走 constants/tokens.js；
#    index.css 与 Login/index.html 的首帧兜底为白名单文件）
bad_hex=$(grep -rnE "#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b" views/ layout/ composables/ constants/ 2>/dev/null \
  | grep -v "constants/tokens.js" || true)
if [ -n "$bad_hex" ]; then
  echo "✗ 视图层硬编码颜色（请用 --am-* token 或 AM_COLORS）："; echo "$bad_hex"; fail=1
fi

# 4. 断点统一 767.98 / 1023.98，避免 CSS 与 JS 双源漂移
bad_bp=$(grep -rnE "@media \(max-width: ?(767|768|1023|1024)px\)" views/ layout/ styles/ 2>/dev/null || true)
if [ -n "$bad_bp" ]; then
  echo "✗ 断点魔法数字（统一 767.98px / 1023.98px，见 constants/breakpoints.js）："; echo "$bad_bp"; fail=1
fi

if [ "$fail" -eq 0 ]; then echo "✓ UI token 约束全部通过"; fi
exit $fail
