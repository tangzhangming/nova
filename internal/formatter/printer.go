package formatter

import (
	"sort"
	"strings"

	"github.com/tangzhangming/nova/internal/ast"
)

// Printer AST 打印器
type Printer struct {
	options *Options
	buf     strings.Builder
	indent  int
	line    int
	col     int
}

// NewPrinter 创建打印器
func NewPrinter(options *Options) *Printer {
	return &Printer{
		options: options,
		indent:  0,
		line:    1,
		col:     0,
	}
}

// Print 打印 AST 并返回格式化的代码
func (p *Printer) Print(file *ast.File) string {
	p.printFile(file)

	result := p.buf.String()

	// 移除行尾空格
	if p.options.RemoveTrailingSpace {
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t")
		}
		result = strings.Join(lines, "\n")
	}

	// 确保文件末尾有换行符
	if p.options.EnsureNewlineAtEOF && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return result
}

// printFile 打印文件
func (p *Printer) printFile(file *ast.File) {
	// 打印命名空间
	if file.Namespace != nil {
		p.write("namespace ")
		p.write(file.Namespace.Name)
		p.writeln()
		p.writeln()
	}

	// 打印 use 声明
	if len(file.Uses) > 0 {
		uses := file.Uses
		if p.options.SortImports {
			uses = p.sortUses(file.Uses)
		}

		for _, use := range uses {
			p.printUse(use)
		}
		p.writeln()
	}

	// 打印声明
	for i, decl := range file.Declarations {
		if i > 0 {
			p.writeln()
		}
		p.printDeclaration(decl)
	}

	// 打印顶层语句
	if len(file.Statements) > 0 {
		if len(file.Declarations) > 0 {
			p.writeln()
		}
		for _, stmt := range file.Statements {
			p.printStatement(stmt)
		}
	}
}

// printUse 打印 use 声明
func (p *Printer) printUse(use *ast.UseDecl) {
	p.write("use ")
	p.write(use.Path)
	if use.Alias != nil {
		p.write(" as ")
		p.write(use.Alias.Name)
	}
	p.writeln(";")
}

// sortUses 排序 use 声明
func (p *Printer) sortUses(uses []*ast.UseDecl) []*ast.UseDecl {
	result := make([]*ast.UseDecl, len(uses))
	copy(result, uses)

	if p.options.GroupImports {
		// 按命名空间分组
		groups := make(map[string][]*ast.UseDecl)
		for _, use := range result {
			parts := strings.Split(use.Path, ".")
			if len(parts) > 0 {
				group := parts[0]
				groups[group] = append(groups[group], use)
			} else {
				groups[""] = append(groups[""], use)
			}
		}

		// 获取组名并排序
		groupNames := make([]string, 0, len(groups))
		for group := range groups {
			groupNames = append(groupNames, group)
		}
		sort.Strings(groupNames)

		// 重新组合
		result = result[:0]
		for _, groupName := range groupNames {
			groupUses := groups[groupName]
			sort.Slice(groupUses, func(i, j int) bool {
				return groupUses[i].Path < groupUses[j].Path
			})
			result = append(result, groupUses...)
		}
	} else {
		// 简单按路径排序
		sort.Slice(result, func(i, j int) bool {
			return result[i].Path < result[j].Path
		})
	}

	return result
}

// printDeclaration 打印声明
func (p *Printer) printDeclaration(decl ast.Declaration) {
	switch d := decl.(type) {
	case *ast.ClassDecl:
		p.printClass(d)
	case *ast.InterfaceDecl:
		p.printInterface(d)
	case *ast.EnumDecl:
		p.printEnum(d)
	case *ast.TypeAliasDecl:
		p.printTypeAlias(d)
	case *ast.NewTypeDecl:
		p.printNewType(d)
	}
}

// 辅助方法

func (p *Printer) write(s string) {
	p.buf.WriteString(s)
	p.col += len(s)
}

func (p *Printer) writeln(s ...string) {
	for _, str := range s {
		p.buf.WriteString(str)
	}
	p.buf.WriteString("\n")
	p.line++
	p.col = 0
}

func (p *Printer) writeIndent() {
	if p.options.IndentStyle == "tabs" {
		p.buf.WriteString(strings.Repeat("\t", p.indent))
	} else {
		p.buf.WriteString(strings.Repeat(" ", p.indent*p.options.IndentSize))
	}
	p.col = p.indent * p.options.IndentSize
}

func (p *Printer) openBrace() {
	if p.options.NewlineBeforeBrace {
		p.writeln()
		p.writeIndent()
		p.write("{")
	} else {
		// K&R 风格：开括号前一个空格，不换行
		p.write(" {")
	}
	p.writeln()
	p.indent++
}

func (p *Printer) closeBrace() {
	p.indent--
	p.writeIndent()
	p.write("}")
}

func (p *Printer) writeSpace() {
	p.write(" ")
}

func (p *Printer) writeOperator(op string) {
	if p.options.SpaceAroundOps {
		p.write(" ")
	}
	p.write(op)
	if p.options.SpaceAroundOps {
		p.write(" ")
	}
}

// ============================================================================
// 类型节点打印
// ============================================================================

func (p *Printer) printType(t ast.TypeNode) {
	switch typ := t.(type) {
	case *ast.SimpleType:
		p.write(typ.Name)
	case *ast.NullableType:
		p.write("?")
		p.printType(typ.Inner)
	case *ast.ArrayType:
		p.printType(typ.ElementType)
		p.write("[")
		if typ.Size != nil {
			p.printExpression(typ.Size)
		}
		p.write("]")
	case *ast.MapType:
		p.write("map[")
		p.printType(typ.KeyType)
		p.write("]")
		p.printType(typ.ValueType)
	case *ast.FuncType:
		p.write("func(")
		for i, param := range typ.Params {
			if i > 0 {
				p.write(", ")
			}
			p.printType(param)
		}
		p.write(")")
		if typ.ReturnType != nil {
			p.write(": ")
			p.printType(typ.ReturnType)
		}
	case *ast.TupleType:
		p.write("(")
		for i, t := range typ.Types {
			if i > 0 {
				p.write(", ")
			}
			p.printType(t)
		}
		p.write(")")
	case *ast.UnionType:
		for i, t := range typ.Types {
			if i > 0 {
				p.write(" | ")
			}
			p.printType(t)
		}
	case *ast.ClassType:
		p.write(typ.Name.Literal)
	case *ast.NullType:
		p.write("null")
	case *ast.GenericType:
		p.printType(typ.BaseType)
		p.write("<")
		for i, arg := range typ.TypeArgs {
			if i > 0 {
				p.write(", ")
			}
			p.printType(arg)
		}
		p.write(">")
	case *ast.TypeParameter:
		p.write(typ.Name.Name)
		if typ.Constraint != nil {
			p.write(" extends ")
			p.printType(typ.Constraint)
		}
		if len(typ.ImplementsTypes) > 0 {
			p.write(" implements ")
			for i, iface := range typ.ImplementsTypes {
				if i > 0 {
					p.write(", ")
				}
				p.printType(iface)
			}
		}
	}
}

// ============================================================================
// 表达式节点打印
// ============================================================================

func (p *Printer) printExpression(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		p.write(e.Token.Literal)
	case *ast.FloatLiteral:
		p.write(e.Token.Literal)
	case *ast.StringLiteral:
		p.write(`"`)
		p.write(e.Value)
		p.write(`"`)
	case *ast.InterpStringLiteral:
		p.write("#\"")
		for _, part := range e.Parts {
			switch part := part.(type) {
			case *ast.StringLiteral:
				p.write(part.Value)
			default:
				p.write("{")
				p.printExpression(part)
				p.write("}")
			}
		}
		p.write("\"")
	case *ast.BoolLiteral:
		if e.Value {
			p.write("true")
		} else {
			p.write("false")
		}
	case *ast.NullLiteral:
		p.write("null")
	case *ast.Variable:
		p.write("$")
		p.write(e.Name)
	case *ast.Identifier:
		p.write(e.Name)
	case *ast.ThisExpr:
		p.write("$this")
	case *ast.SelfExpr:
		p.write("self")
	case *ast.ParentExpr:
		p.write("parent")
	case *ast.ArrayLiteral:
		if e.ElementType != nil {
			p.printType(e.ElementType)
		}
		p.write("{")
		for i, elem := range e.Elements {
			if i > 0 {
				p.write(", ")
			}
			p.printExpression(elem)
		}
		p.write("}")
	case *ast.MapLiteral:
		if e.MapToken.Type != 0 {
			p.write("map[")
			p.printType(e.KeyType)
			p.write("]")
			p.printType(e.ValueType)
		}
		p.write("{")
		for i, pair := range e.Pairs {
			if i > 0 {
				p.write(", ")
			}
			p.printExpression(pair.Key)
			p.write(": ")
			p.printExpression(pair.Value)
		}
		p.write("}")
	case *ast.SuperArrayLiteral:
		p.write("[")
		for i, elem := range e.Elements {
			if i > 0 {
				p.write(", ")
			}
			if elem.Key != nil {
				p.printExpression(elem.Key)
				p.write(" => ")
			}
			p.printExpression(elem.Value)
		}
		p.write("]")
	case *ast.UnaryExpr:
		if e.Prefix {
			p.write(e.Operator.Literal)
			p.printExpression(e.Operand)
		} else {
			p.printExpression(e.Operand)
			p.write(e.Operator.Literal)
		}
	case *ast.BinaryExpr:
		p.printExpression(e.Left)
		p.writeOperator(e.Operator.Literal)
		p.printExpression(e.Right)
	case *ast.IsExpr:
		p.printExpression(e.Expr)
		if e.Negated {
			p.write(" !is ")
		} else {
			p.write(" is ")
		}
		p.printType(e.TypeName)
	case *ast.TernaryExpr:
		p.printExpression(e.Condition)
		p.write(" ? ")
		p.printExpression(e.Then)
		p.write(" : ")
		p.printExpression(e.Else)
	case *ast.AssignExpr:
		p.printExpression(e.Left)
		p.writeOperator(e.Operator.Literal)
		p.printExpression(e.Right)
	case *ast.CallExpr:
		p.printExpression(e.Function)
		p.write("(")
		for i, arg := range e.Arguments {
			if i > 0 {
				p.write(", ")
			}
			p.printExpression(arg)
		}
		for i, na := range e.NamedArguments {
			if len(e.Arguments) > 0 || i > 0 {
				p.write(", ")
			}
			p.write(na.Name.Name)
			p.write(": ")
			p.printExpression(na.Value)
		}
		p.write(")")
	case *ast.IndexExpr:
		p.printExpression(e.Object)
		p.write("[")
		p.printExpression(e.Index)
		p.write("]")
	case *ast.PropertyAccess:
		p.printExpression(e.Object)
		p.write("->")
		p.write(e.Property.Name)
	case *ast.MethodCall:
		p.printExpression(e.Object)
		p.write("->")
		p.write(e.Method.Name)
		p.write("(")
		for i, arg := range e.Arguments {
			if i > 0 {
				p.write(", ")
			}
			p.printExpression(arg)
		}
		for i, na := range e.NamedArguments {
			if len(e.Arguments) > 0 || i > 0 {
				p.write(", ")
			}
			p.write(na.Name.Name)
			p.write(": ")
			p.printExpression(na.Value)
		}
		p.write(")")
	case *ast.StaticAccess:
		// Class 可能是 Identifier, SelfExpr, ParentExpr
		if ident, ok := e.Class.(*ast.Identifier); ok {
			p.write(ident.Name)
		} else if _, ok := e.Class.(*ast.SelfExpr); ok {
			p.write("self")
		} else if _, ok := e.Class.(*ast.ParentExpr); ok {
			p.write("parent")
		} else {
			p.printExpression(e.Class)
		}
		p.write("::")
		// Member 可能是 Identifier, Variable, CallExpr
		if ident, ok := e.Member.(*ast.Identifier); ok {
			p.write(ident.Name)
		} else if varExpr, ok := e.Member.(*ast.Variable); ok {
			p.write("$")
			p.write(varExpr.Name)
		} else {
			p.printExpression(e.Member)
		}
	case *ast.ClassAccessExpr:
		p.printExpression(e.Object)
		p.write("::class")
	case *ast.NewExpr:
		p.write("new ")
		p.write(e.ClassName.Name)
		if len(e.TypeArgs) > 0 {
			p.write("<")
			for i, arg := range e.TypeArgs {
				if i > 0 {
					p.write(", ")
				}
				p.printType(arg)
			}
			p.write(">")
		}
		if len(e.Arguments) > 0 || len(e.NamedArguments) > 0 {
			p.write("(")
			for i, arg := range e.Arguments {
				if i > 0 {
					p.write(", ")
				}
				p.printExpression(arg)
			}
			for i, na := range e.NamedArguments {
				if len(e.Arguments) > 0 || i > 0 {
					p.write(", ")
				}
				p.write(na.Name.Name)
				p.write(": ")
				p.printExpression(na.Value)
			}
			p.write(")")
		}
	case *ast.ClosureExpr:
		p.write("function(")
		for i, param := range e.Parameters {
			if i > 0 {
				p.write(", ")
			}
			p.printParameter(param)
		}
		p.write(")")
		if e.ReturnType != nil {
			p.write(": ")
			p.printType(e.ReturnType)
		}
		if len(e.UseVars) > 0 {
			p.write(" use (")
			for i, v := range e.UseVars {
				if i > 0 {
					p.write(", ")
				}
				p.write("$")
				p.write(v.Name)
			}
			p.write(")")
		}
		p.printBlock(e.Body)
	case *ast.ArrowFuncExpr:
		p.write("(")
		for i, param := range e.Parameters {
			if i > 0 {
				p.write(", ")
			}
			p.printParameter(param)
		}
		p.write(")")
		if e.ReturnType != nil {
			p.write(": ")
			p.printType(e.ReturnType)
		}
		p.write(" => ")
		p.printExpression(e.Body)
	case *ast.TypeCastExpr:
		p.printExpression(e.Expr)
		if e.Safe {
			p.write(" as? ")
		} else {
			p.write(" as ")
		}
		p.printType(e.TargetType)
	case *ast.MatchExpr:
		p.write("match (")
		p.printExpression(e.Expr)
		p.write(") ")
		p.openBrace()
		for i, matchCase := range e.Cases {
			if i > 0 {
				p.writeln()
			}
			p.writeIndent()
			p.printPattern(matchCase.Pattern)
			if matchCase.Guard != nil {
				p.write(" if ")
				p.printExpression(matchCase.Guard)
			}
			p.write(" => ")
			p.printExpression(matchCase.Body)
			if i < len(e.Cases)-1 {
				p.write(",")
			}
		}
		p.writeln()
		p.closeBrace()

	case *ast.SwitchExpr:
		p.write("switch (")
		p.printExpression(e.Expr)
		p.write(")")
		p.openBrace()
		for i, switchCase := range e.Cases {
			p.writeIndent()
			p.write("case ")
			// 打印多个值：case 1, 2, 3
			for j, value := range switchCase.Values {
				if j > 0 {
					p.write(", ")
				}
				p.printExpression(value)
			}
			// SwitchExpr 必须使用 => 形式
			if expr, ok := switchCase.Body.(ast.Expression); ok {
				p.write(" => ")
				p.printExpression(expr)
				if i < len(e.Cases)-1 || e.Default != nil {
					p.write(",")
				}
				p.writeln()
			}
		}
		if e.Default != nil {
			p.writeIndent()
			if expr, ok := e.Default.Body.(ast.Expression); ok {
				p.write("default => ")
				p.printExpression(expr)
				p.writeln()
			}
		}
		p.closeBrace()
	}
}

func (p *Printer) printPattern(pattern ast.Pattern) {
	switch pat := pattern.(type) {
	case *ast.TypePattern:
		p.printType(pat.Type)
		if pat.Variable != nil {
			p.write(" $")
			p.write(pat.Variable.Name)
		}
	case *ast.ValuePattern:
		p.printExpression(pat.Value)
	case *ast.WildcardPattern:
		p.write("_")
	}
}

func (p *Printer) printParameter(param *ast.Parameter) {
	if param.Type != nil {
		p.printType(param.Type)
		p.write(" ")
	}
	if param.Variadic {
		p.write("...")
	}
	p.write("$")
	p.write(param.Name.Name)
	if param.Default != nil {
		p.write(" = ")
		p.printExpression(param.Default)
	}
}

// ============================================================================
// 语句节点打印
// ============================================================================

func (p *Printer) printStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		p.writeIndent()
		p.printExpression(s.Expr)
		p.writeln(";")
	case *ast.VarDeclStmt:
		p.writeIndent()
		if s.Type != nil {
			p.printType(s.Type)
			p.write(" ")
		}
		p.write("$")
		p.write(s.Name.Name)
		if s.Value != nil {
			if s.Type != nil {
				p.write(" = ")
			} else {
				p.write(" := ")
			}
			p.printExpression(s.Value)
		}
		p.writeln(";")
	case *ast.MultiVarDeclStmt:
		p.writeIndent()
		for i, name := range s.Names {
			if i > 0 {
				p.write(", ")
			}
			p.write("$")
			p.write(name.Name)
		}
		p.writeOperator(s.Operator.Literal)
		p.printExpression(s.Value)
		p.writeln(";")
	case *ast.BlockStmt:
		p.openBrace()
		for _, stmt := range s.Statements {
			p.printStatement(stmt)
		}
		p.closeBrace()
	case *ast.IfStmt:
		p.writeIndent()
		p.write("if (")
		p.printExpression(s.Condition)
		p.write(")")
		p.openBrace()
		for _, stmt := range s.Then.Statements {
			p.printStatement(stmt)
		}
		if len(s.ElseIfs) > 0 || s.Else != nil {
			p.closeBraceInline()
		} else {
			p.closeBrace()
		}
		for _, elseif := range s.ElseIfs {
			p.write(" elseif (")
			p.printExpression(elseif.Condition)
			p.write(")")
			p.openBrace()
			for _, stmt := range elseif.Body.Statements {
				p.printStatement(stmt)
			}
			if s.Else != nil || elseif != s.ElseIfs[len(s.ElseIfs)-1] {
				p.closeBraceInline()
			} else {
				p.closeBrace()
			}
		}
		if s.Else != nil {
			p.write(" else")
			p.printBlock(s.Else)
		}
		p.writeln()
	case *ast.SwitchStmt:
		p.writeIndent()
		p.write("switch (")
		p.printExpression(s.Expr)
		p.write(")")
		p.openBrace()
		for _, switchCase := range s.Cases {
			p.writeIndent()
			p.write("case ")
			// 打印多个值：case 1, 2, 3
			for i, value := range switchCase.Values {
				if i > 0 {
					p.write(", ")
				}
				p.printExpression(value)
			}
			
			// 检查是 => 还是 : 形式
			if expr, ok := switchCase.Body.(ast.Expression); ok {
				// => 形式
				p.write(" => ")
				p.printExpression(expr)
				p.writeln(",")
			} else if stmts, ok := switchCase.Body.([]ast.Statement); ok {
				// : 形式
				p.writeln(":")
				p.indent++
				for _, stmt := range stmts {
					p.printStatement(stmt)
				}
				p.indent--
			}
		}
		if s.Default != nil {
			p.writeIndent()
			if expr, ok := s.Default.Body.(ast.Expression); ok {
				// => 形式
				p.write("default => ")
				p.printExpression(expr)
				p.writeln()
			} else if stmts, ok := s.Default.Body.([]ast.Statement); ok {
				// : 形式
				p.writeln("default:")
				p.indent++
				for _, stmt := range stmts {
					p.printStatement(stmt)
				}
				p.indent--
			}
		}
		p.closeBrace()
		p.writeln()
	case *ast.ForStmt:
		p.writeIndent()
		p.write("for (")
		if s.Init != nil {
			p.printStatementInline(s.Init)
		}
		p.write("; ")
		if s.Condition != nil {
			p.printExpression(s.Condition)
		}
		p.write("; ")
		if s.Post != nil {
			p.printExpression(s.Post)
		}
		p.write(")")
		p.printBlock(s.Body)
		p.writeln()
	case *ast.ForeachStmt:
		p.writeIndent()
		p.write("foreach (")
		p.printExpression(s.Iterable)
		p.write(" as ")
		if s.Key != nil {
			p.write("$")
			p.write(s.Key.Name)
			p.write(" => ")
		}
		p.write("$")
		p.write(s.Value.Name)
		p.write(")")
		p.printBlock(s.Body)
		p.writeln()
	case *ast.WhileStmt:
		p.writeIndent()
		p.write("while (")
		p.printExpression(s.Condition)
		p.write(")")
		p.printBlock(s.Body)
		p.writeln()
	case *ast.DoWhileStmt:
		p.writeIndent()
		p.write("do")
		p.printBlock(s.Body)
		p.write(" while (")
		p.printExpression(s.Condition)
		p.writeln(");")
	case *ast.BreakStmt:
		p.writeIndent()
		p.writeln("break;")
	case *ast.ContinueStmt:
		p.writeIndent()
		p.writeln("continue;")
	case *ast.ReturnStmt:
		p.writeIndent()
		p.write("return")
		if len(s.Values) > 0 {
			p.write(" ")
			for i, v := range s.Values {
				if i > 0 {
					p.write(", ")
				}
				p.printExpression(v)
			}
		}
		p.writeln(";")
	case *ast.TryStmt:
		p.writeIndent()
		p.write("try")
		p.printBlock(s.Try)
		for _, catch := range s.Catches {
			p.write(" catch (")
			p.printType(catch.Type)
			p.write(" $")
			p.write(catch.Variable.Name)
			p.write(")")
			p.printBlock(catch.Body)
		}
		if s.Finally != nil {
			p.write(" finally")
			p.printBlock(s.Finally.Body)
		}
		p.writeln()
	case *ast.ThrowStmt:
		p.writeIndent()
		p.write("throw ")
		p.printExpression(s.Exception)
		p.writeln(";")
	case *ast.EchoStmt:
		p.writeIndent()
		p.write("echo ")
		p.printExpression(s.Value)
		p.writeln(";")
	}
}

func (p *Printer) printStatementInline(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		if s.Type != nil {
			p.printType(s.Type)
			p.write(" ")
		}
		p.write("$")
		p.write(s.Name.Name)
		if s.Value != nil {
			if s.Type != nil {
				p.write(" = ")
			} else {
				p.write(" := ")
			}
			p.printExpression(s.Value)
		}
	case *ast.ExprStmt:
		p.printExpression(s.Expr)
	}
}

func (p *Printer) printBlock(block *ast.BlockStmt) {
	p.openBrace()
	for _, stmt := range block.Statements {
		p.printStatement(stmt)
	}
	p.closeBrace()
}

func (p *Printer) closeBraceInline() {
	p.indent--
	p.write("}")
}

// ============================================================================
// 声明节点打印
// ============================================================================

func (p *Printer) printAnnotation(ann *ast.Annotation) {
	p.write("@")
	p.write(ann.Name.Name)
	if len(ann.Args) > 0 || len(ann.NamedArgs) > 0 {
		p.write("(")
		first := true
		// 先打印位置参数
		for _, arg := range ann.Args {
			if !first {
				p.write(", ")
			}
			first = false
			p.printExpression(arg)
		}
		// 再打印命名参数
		for name, arg := range ann.NamedArgs {
			if !first {
				p.write(", ")
			}
			first = false
			p.write(name)
			p.write(" = ")
			p.printExpression(arg)
		}
		p.write(")")
	}
}

func (p *Printer) printClass(class *ast.ClassDecl) {
	// 打印注解
	for _, ann := range class.Annotations {
		p.writeIndent()
		p.printAnnotation(ann)
		p.writeln()
	}

	// 打印修饰符
	p.writeIndent()
	if class.Visibility != ast.VisibilityDefault {
		p.write(class.Visibility.String())
		p.write(" ")
	}
	if class.Abstract {
		p.write("abstract ")
	}
	if class.Final {
		p.write("final ")
	}

	p.write("class ")
	p.write(class.Name.Name)

	// 泛型参数
	if len(class.TypeParams) > 0 {
		p.write("<")
		for i, tp := range class.TypeParams {
			if i > 0 {
				p.write(", ")
			}
			p.printTypeParameter(tp)
		}
		p.write(">")
	}

	// 继承
	if class.Extends != nil {
		p.write(" extends ")
		p.write(class.Extends.Name)
	}

	// 实现接口
	if len(class.Implements) > 0 {
		p.write(" implements ")
		for i, iface := range class.Implements {
			if i > 0 {
		p.write(", ")
		}
		p.printType(iface)
	}
	}

	p.openBrace()

	// 打印常量
	for i, constDecl := range class.Constants {
		if i > 0 {
			p.writeln()
		}
		p.printConst(constDecl)
	}

	// 打印属性
	if len(class.Constants) > 0 && len(class.Properties) > 0 {
		p.writeln()
	}
	for i, prop := range class.Properties {
		if i > 0 {
			p.writeln()
		}
		p.printProperty(prop)
	}

	// 打印方法
	if (len(class.Constants) > 0 || len(class.Properties) > 0) && len(class.Methods) > 0 {
		p.writeln()
	}
	for i, method := range class.Methods {
		if i > 0 {
			p.writeln()
		}
		p.printMethod(method)
	}

	p.closeBrace()
	p.writeln()
}

func (p *Printer) printTypeParameter(tp *ast.TypeParameter) {
	p.write(tp.Name.Name)
	if tp.Constraint != nil {
		p.write(" extends ")
		p.printType(tp.Constraint)
	}
	if len(tp.ImplementsTypes) > 0 {
		p.write(" implements ")
		for i, iface := range tp.ImplementsTypes {
			if i > 0 {
				p.write(", ")
			}
			p.printType(iface)
		}
	}
}

func (p *Printer) printInterface(iface *ast.InterfaceDecl) {
	// 打印注解
	for _, ann := range iface.Annotations {
		p.writeIndent()
		p.printAnnotation(ann)
		p.writeln()
	}

	// 打印修饰符
	p.writeIndent()
	if iface.Visibility != ast.VisibilityDefault {
		p.write(iface.Visibility.String())
		p.write(" ")
	}

	p.write("interface ")
	p.write(iface.Name.Name)

	// 泛型参数
	if len(iface.TypeParams) > 0 {
		p.write("<")
		for i, tp := range iface.TypeParams {
			if i > 0 {
				p.write(", ")
			}
			p.printTypeParameter(tp)
		}
		p.write(">")
	}

	// 继承接口
	if len(iface.Extends) > 0 {
		p.write(" extends ")
		for i, ext := range iface.Extends {
			if i > 0 {
				p.write(", ")
			}
			p.printType(ext)
		}
	}

	p.openBrace()

	// 打印方法
	for i, method := range iface.Methods {
		if i > 0 {
			p.writeln()
		}
		p.printMethod(method)
	}

	p.closeBrace()
	p.writeln()
}

func (p *Printer) printEnum(enum *ast.EnumDecl) {
	p.writeIndent()
	p.write("enum ")
	p.write(enum.Name.Name)
	if enum.Type != nil {
		p.write(": ")
		p.printType(enum.Type)
	}
	p.openBrace()

	for i, enumCase := range enum.Cases {
		if i > 0 {
			p.writeln()
		}
		p.writeIndent()
		p.write(enumCase.Name.Name)
		if enumCase.Value != nil {
			p.write(" = ")
			p.printExpression(enumCase.Value)
		}
		if i < len(enum.Cases)-1 {
			p.write(",")
		}
	}

	p.writeln()
	p.closeBrace()
	p.writeln()
}

func (p *Printer) printMethod(method *ast.MethodDecl) {
	// 打印注解
	for _, ann := range method.Annotations {
		p.writeIndent()
		p.printAnnotation(ann)
		p.writeln()
	}

	// 打印修饰符
	p.writeIndent()
	if method.Visibility != ast.VisibilityDefault {
		p.write(method.Visibility.String())
		p.write(" ")
	}
	if method.Static {
		p.write("static ")
	}
	if method.Abstract {
		p.write("abstract ")
	}
	if method.Final {
		p.write("final ")
	}

	p.write("function ")
	p.write(method.Name.Name)

	// 泛型参数
	if len(method.TypeParams) > 0 {
		p.write("<")
		for i, tp := range method.TypeParams {
			if i > 0 {
				p.write(", ")
			}
			p.printTypeParameter(tp)
		}
		p.write(">")
	}

	p.write("(")
	for i, param := range method.Parameters {
		if i > 0 {
			p.write(", ")
		}
		p.printParameter(param)
	}
	p.write(")")

	if method.ReturnType != nil {
		p.write(": ")
		p.printType(method.ReturnType)
	}

	if method.Body != nil {
		p.write(" ")
		p.printBlock(method.Body)
	} else {
		p.writeln(";")
	}
	p.writeln()
}

func (p *Printer) printProperty(prop *ast.PropertyDecl) {
	// 打印注解
	for _, ann := range prop.Annotations {
		p.writeIndent()
		p.printAnnotation(ann)
		p.writeln()
	}

	// 打印修饰符
	p.writeIndent()
	if prop.Visibility != ast.VisibilityDefault {
		p.write(prop.Visibility.String())
		p.write(" ")
	}
	if prop.Static {
		p.write("static ")
	}
	if prop.Final {
		p.write("final ")
	}

	p.printType(prop.Type)
	p.write(" ")
	p.write("$")
	p.write(prop.Name.Name)

	// 表达式体属性
	if prop.ExprBody != nil {
		p.write(" => ")
		p.printExpression(prop.ExprBody)
		p.writeln(";")
		return
	}

	// 属性访问器
	if prop.Accessor != nil {
		p.write(" ")
		p.printPropertyAccessor(prop.Accessor)
		return
	}

	// 普通字段
	if prop.Value != nil {
		p.write(" = ")
		p.printExpression(prop.Value)
	}
	p.writeln(";")
}

func (p *Printer) printPropertyAccessor(accessor *ast.PropertyAccessor) {
	p.write("{")
	if accessor.GetToken.Type != 0 {
		p.write(" get")
		if accessor.GetVis != ast.VisibilityDefault {
			p.write(" ")
			p.write(accessor.GetVis.String())
		}
		if accessor.GetBody != nil {
			p.printBlock(accessor.GetBody)
		} else if accessor.GetExpr != nil {
			p.write(" => ")
			p.printExpression(accessor.GetExpr)
		} else {
			p.write(";")
		}
	}
	if accessor.SetToken.Type != 0 {
		if accessor.GetToken.Type != 0 {
			p.write(" ")
		}
		p.write("set")
		if accessor.SetVis != ast.VisibilityDefault {
			p.write(" ")
			p.write(accessor.SetVis.String())
		}
		if accessor.SetBody != nil {
			p.printBlock(accessor.SetBody)
		} else if accessor.SetExpr != nil {
			p.write(" => ")
			p.printExpression(accessor.SetExpr)
		} else {
			p.write(";")
		}
	}
	p.write(" }")
	p.writeln()
}

func (p *Printer) printConst(constDecl *ast.ConstDecl) {
	// 打印注解
	for _, ann := range constDecl.Annotations {
		p.writeIndent()
		p.printAnnotation(ann)
		p.writeln()
	}

	// 打印修饰符
	p.writeIndent()
	if constDecl.Visibility != ast.VisibilityDefault {
		p.write(constDecl.Visibility.String())
		p.write(" ")
	}

	p.write("const ")
	p.printType(constDecl.Type)
	p.write(" ")
	p.write(constDecl.Name.Name)
	p.write(" = ")
	p.printExpression(constDecl.Value)
	p.writeln(";")
}

func (p *Printer) printTypeAlias(alias *ast.TypeAliasDecl) {
	p.writeIndent()
	p.write("type ")
	p.write(alias.Name.Name)
	p.write(" = ")
	p.printType(alias.AliasType)
	p.writeln(";")
}

func (p *Printer) printNewType(newType *ast.NewTypeDecl) {
	p.writeIndent()
	p.write("type ")
	p.write(newType.Name.Name)
	p.write(" ")
	p.printType(newType.BaseType)
	p.writeln()
}

