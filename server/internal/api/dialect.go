package api

import (
	"fmt"

	"github.com/automedic/automedic/internal/store"
)

// 手写 SQL 的方言适配：postgres 与 mysql/sqlite 在布尔求和、日期截取、类型转换上语法不同

func isPG() bool { return store.Dialect == "postgres" }

// sumEq 统计满足 col = val 的行数
func sumEq(col, val string) string {
	if isPG() {
		return fmt.Sprintf("sum(case when %s = '%s' then 1 else 0 end)", col, val)
	}
	return fmt.Sprintf("sum(%s='%s')", col, val)
}

// sumIn 统计 col 属于 vals 的行数
func sumIn(col string, vals []string) string {
	if isPG() {
		cond := ""
		for i, v := range vals {
			if i > 0 {
				cond += " OR "
			}
			cond += fmt.Sprintf("%s = '%s'", col, v)
		}
		return fmt.Sprintf("sum(case when %s then 1 else 0 end)", cond)
	}
	list := ""
	for i, v := range vals {
		if i > 0 {
			list += ","
		}
		list += fmt.Sprintf("'%s'", v)
	}
	return fmt.Sprintf("sum(%s IN (%s))", col, list)
}

// dayExpr 把时间列转成 YYYY-MM-DD 字符串
func dayExpr(col string) string {
	if isPG() {
		return fmt.Sprintf("to_char(%s, 'YYYY-MM-DD')", col)
	}
	return fmt.Sprintf("substr(%s,1,10)", col)
}

// castText 数值列转字符串
func castText(expr string) string {
	if isPG() {
		return expr + "::text"
	}
	return "CAST(" + expr + " AS CHAR)"
}

// roundAvg 平均耗时取整
func roundAvg(col string) string {
	if isPG() {
		return fmt.Sprintf("COALESCE(round(avg(%s)),0)", col)
	}
	return fmt.Sprintf("COALESCE(round(avg(%s)),0)", col)
}
