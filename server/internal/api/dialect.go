package api

import (
	"fmt"
	"strings"
)

// 手写 SQL 按 PostgreSQL 方言编写。

func sumEq(col, val string) string {
	return fmt.Sprintf("sum(case when %s = '%s' then 1 else 0 end)", col, val)
}

func sumIn(col string, vals []string) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, fmt.Sprintf("%s = '%s'", col, v))
	}
	return fmt.Sprintf("sum(case when %s then 1 else 0 end)", strings.Join(parts, " OR "))
}

func dayExpr(col string) string {
	return fmt.Sprintf("to_char(%s, 'YYYY-MM-DD')", col)
}

func castText(expr string) string {
	return expr + "::text"
}

func roundAvg(col string) string {
	return fmt.Sprintf("COALESCE(round(avg(%s)),0)", col)
}
