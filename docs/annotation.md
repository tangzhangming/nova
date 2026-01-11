# Sola 注解系统设计

## 概述

将 Sola 的注解从**弱类型（纯元数据）**升级为 **强类型（注解是类）**，采用 **PHP 8 Attributes** 风格设计。

### 设计目标

1. **类型安全**：注解名拼错、参数错误在编译期报错
2. **IDE 友好**：支持自动补全、跳转定义、重构
3. **ORM 支持**：满足 ORM 框架对注解的需求
4. **向后兼容**：平滑迁移，不破坏现有代码
5. **语法统一**：注解就是类，不引入新关键字

---

## 一、注解定义语法

### 1.1 基本定义

注解是普通类，使用 `@Attribute` 元注解标记：

```sola
namespace sola.orm

use sola.annotation.Attribute;

// 简单注解（无参数）
@Attribute
public class Entity {
}

// 带参数的注解 - 通过构造函数定义参数
@Attribute
public class Table {
    public string $name;
    public string $schema;
    
    public function __construct(string $name, string $schema = "public") {
        $this->name = $name;
        $this->schema = $schema;
    }
}

// 多参数注解
@Attribute
public class Column {
    public string $name;
    public string $type;
    public bool $nullable;
    public bool $primaryKey;
    public string $default;
    
    public function __construct(
        string $name = "",
        string $type = "auto",
        bool $nullable = true,
        bool $primaryKey = false,
        string $default = ""
    ) {
        $this->name = $name;
        $this->type = $type;
        $this->nullable = $nullable;
        $this->primaryKey = $primaryKey;
        $this->default = $default;
    }
}
```

### 1.2 设计原则

- **注解就是类**：使用 `class` 定义，不引入新关键字
- **参数即构造函数参数**：通过构造函数定义注解接受的参数
- **`@Attribute` 标记**：编译器通过此元注解识别哪些类可以作为注解使用
- **与 PHP 8 一致**：PHP 开发者零学习成本

### 1.3 参数类型限制

注解构造函数参数只能是以下类型（编译期常量）：

| 类型 | 说明 | 示例 |
|------|------|------|
| `string` | 字符串 | `"users"` |
| `int` | 整数 | `100` |
| `float` | 浮点数 | `3.14` |
| `bool` | 布尔值 | `true`, `false` |
| `string[]` | 字符串数组 | `["a", "b"]` |
| `int[]` | 整数数组 | `[1, 2, 3]` |
| 枚举类型 | 已定义的枚举 | `FetchMode::LAZY` |

**不支持**：对象实例、动态类型、运行时表达式

---

## 二、元注解（Meta-Annotations）

元注解用于约束注解的使用方式，定义在 `sola.annotation` 命名空间。

### 2.1 @Attribute - 标记类为注解

```sola
namespace sola.annotation

// Attribute 本身也是注解（自举）
@Attribute
public class Attribute {
}
```

只有标记了 `@Attribute` 的类才能作为注解使用。

### 2.2 @Target - 限制注解使用位置

```sola
namespace sola.annotation

public enum ElementType {
    CLASS,       // 类
    INTERFACE,   // 接口
    METHOD,      // 方法
    PROPERTY,    // 属性
    PARAMETER,   // 方法参数
    CONSTRUCTOR, // 构造函数
    ALL          // 所有位置（默认）
}

@Attribute
public class Target {
    public ElementType[] $value;
    
    public function __construct(ElementType[] $value) {
        $this->value = $value;
    }
}
```

**使用示例**：

```sola
use sola.annotation.Attribute;
use sola.annotation.Target;
use sola.annotation.ElementType;

// Column 只能用于属性
@Attribute
@Target([ElementType::PROPERTY])
public class Column {
    public function __construct(public string $name = "") {}
}

// 错误使用 - 编译报错
@Column(name = "id")      // ❌ 错误：Column 不能用于类
public class User { }
```

### 2.3 @Retention - 注解保留策略

```sola
namespace sola.annotation

public enum RetentionPolicy {
    SOURCE,     // 仅源码，编译后丢弃
    COMPILE,    // 编译期可用，运行时不可见
    RUNTIME     // 运行时可用（默认）
}

@Attribute
public class Retention {
    public RetentionPolicy $value;
    
    public function __construct(RetentionPolicy $value = RetentionPolicy::RUNTIME) {
        $this->value = $value;
    }
}
```

### 2.4 @Repeatable - 允许重复使用

```sola
@Attribute
public class Repeatable {
    public string $container;
    
    public function __construct(string $container) {
        $this->container = $container;
    }
}
```

**使用示例**：

```sola
@Attribute
@Repeatable(container = "sola.orm.Indexes")
public class Index {
    public function __construct(
        public string $name,
        public string[] $columns
    ) {}
}

@Attribute
public class Indexes {
    public function __construct(public Index[] $value) {}
}

// 可以重复使用
@Index(name = "idx_name", columns = ["name"])
@Index(name = "idx_email", columns = ["email"])
public class User { }
```

### 2.5 @Inherited - 子类继承注解

```sola
@Attribute
public class Inherited {
}
```

**使用示例**：

```sola
@Attribute
@Inherited
@Target([ElementType::CLASS])
public class Entity {
}

@Entity
public class BaseModel { }

// User 自动继承 @Entity
public class User extends BaseModel { }
```

---

## 三、注解使用语法

### 3.1 基本使用

```sola
use sola.orm.Entity;
use sola.orm.Table;
use sola.orm.Column;

@Entity
@Table("users")
public class User {
    
    @Column(name = "id", primaryKey = true)
    public int $id;
    
    @Column(name = "user_name", nullable = false)
    public string $name;
    
    @Column  // 使用默认值
    public string $email;
}
```

### 3.2 参数传递方式

```sola
// 1. 命名参数（推荐，参数多时更清晰）
@Column(name = "user_id", nullable = false)

// 2. 位置参数（按构造函数参数顺序）
@Table("users")
@Column("user_id", "varchar", false)

// 3. 混合使用（位置参数在前，命名参数在后）
@Column("user_id", nullable = false)
```

### 3.3 数组参数

```sola
@Index(name = "idx_composite", columns = ["name", "email"])

// 单元素数组可以省略方括号
@Target(ElementType::CLASS)  // 等价于 @Target([ElementType::CLASS])
```

### 3.4 无参数注解

```sola
@Entity       // 无参数
@PrimaryKey   // 无参数
public int $id;
```

---

## 四、编译器修改

### 4.1 无需新增关键字

采用 PHP 8 风格后，**不需要新增 `annotation` 关键字**，注解就是普通类。

### 4.2 修改 AST 节点

```go
// internal/ast/ast.go

// Annotation 注解使用（修改现有结构）
type Annotation struct {
    AtToken    token.Token
    Name       *Identifier           // 注解类名（支持完整命名空间）
    Args       []Expression          // 位置参数
    NamedArgs  map[string]Expression // 命名参数（新增）
}
```

### 4.3 解析器修改

```go
// internal/parser/parser.go

func (p *Parser) parseAnnotation() *ast.Annotation {
    atToken := p.advance()  // @
    name := p.parseIdentifier()  // 支持 a.b.ClassName
    
    var args []Expression
    var namedArgs map[string]Expression
    
    if p.check(token.LPAREN) {
        p.advance()
        args, namedArgs = p.parseAnnotationArgs()
        p.consume(token.RPAREN)
    }
    
    return &ast.Annotation{
        AtToken:   atToken,
        Name:      name,
        Args:      args,
        NamedArgs: namedArgs,
    }
}

// 解析注解参数（支持命名参数）
func (p *Parser) parseAnnotationArgs() ([]Expression, map[string]Expression) {
    // @Foo("value")              -> 位置参数
    // @Foo(name = "value")       -> 命名参数
    // @Foo("a", name = "b")      -> 混合（位置在前）
}
```

### 4.4 编译器验证

```go
// internal/compiler/annotation_validator.go

type AnnotationValidator struct {
    classes map[string]*bytecode.Class
}

func (v *AnnotationValidator) Validate(ann *ast.Annotation, target ast.Node) []error {
    var errors []error
    
    // 1. 检查注解类是否存在
    class := v.classes[ann.Name.String()]
    if class == nil {
        errors = append(errors, fmt.Errorf("未定义的注解: %s", ann.Name))
        return errors
    }
    
    // 2. 检查类是否有 @Attribute 标记
    if !v.hasAttribute(class) {
        errors = append(errors, fmt.Errorf("%s 不是注解类（缺少 @Attribute）", ann.Name))
        return errors
    }
    
    // 3. 检查 @Target 是否匹配
    if !v.checkTarget(class, target) {
        errors = append(errors, fmt.Errorf("@%s 不能用于此位置", ann.Name))
    }
    
    // 4. 检查构造函数参数
    errors = append(errors, v.validateArgs(class, ann)...)
    
    // 5. 检查是否重复使用（非 @Repeatable）
    
    return errors
}
```

### 4.5 字节码存储

```go
// internal/bytecode/value.go

// Annotation 编译后的注解实例
type Annotation struct {
    ClassName string            // 注解类的完整名称
    Args      map[string]Value  // 参数名 -> 值
}

// Class 增加字段
type Class struct {
    // ... 现有字段
    IsAttribute bool  // 是否是注解类（有 @Attribute 标记）
}
```

---

## 五、反射 API

### 5.1 标准库封装（`sola.reflect`）

```sola
namespace sola.reflect

public class ClassReflector {
    private string $className;
    
    public static function forClass(string $className): ClassReflector {
        return new ClassReflector($className);
    }
    
    public static function forObject(dynamic $obj): ClassReflector {
        $className := native_reflect_get_class($obj);
        return new ClassReflector($className);
    }
    
    // 获取类上的所有注解
    public function getAnnotations(): Annotation[] {
        // ...
    }
    
    // 获取指定注解
    public function getAnnotation(string $name): Annotation|null {
        // ...
    }
    
    // 检查是否有指定注解
    public function hasAnnotation(string $name): bool {
        // ...
    }
    
    // 获取所有属性
    public function getProperties(): PropertyReflector[] {
        // ...
    }
    
    // 获取指定属性
    public function getProperty(string $name): PropertyReflector|null {
        // ...
    }
}

public class PropertyReflector {
    private string $className;
    private string $propertyName;
    
    public function getName(): string {
        return $this->propertyName;
    }
    
    public function getAnnotations(): Annotation[] {
        // ...
    }
    
    public function getAnnotation(string $name): Annotation|null {
        // ...
    }
    
    public function hasAnnotation(string $name): bool {
        // ...
    }
    
    // 获取/设置值
    public function getValue(dynamic $obj): dynamic {
        // ...
    }
    
    public function setValue(dynamic $obj, dynamic $value): void {
        // ...
    }
}

public class Annotation {
    private string $name;
    private dynamic $args;
    
    public function getName(): string {
        return $this->name;
    }
    
    // 获取参数值
    public function get(string $key): dynamic {
        return $this->args[$key];
    }
    
    // 获取字符串参数
    public function getString(string $key): string {
        return $this->args[$key] as string;
    }
    
    // 获取布尔参数
    public function getBool(string $key): bool {
        return $this->args[$key] as bool;
    }
    
    // 获取整数参数
    public function getInt(string $key): int {
        return $this->args[$key] as int;
    }
}
```

### 5.2 使用示例

```sola
use sola.reflect.ClassReflector;

$reflector := ClassReflector::forClass("app.models.User");

// 获取表名
$tableAnn := $reflector->getAnnotation("Table");
if ($tableAnn != null) {
    $tableName := $tableAnn->getString("name");
    echo "Table: " + $tableName;
}

// 遍历所有带 @Column 的属性
$props := $reflector->getProperties();
for ($i := 0; $i < len($props); $i++) {
    $prop := $props[$i];
    $colAnn := $prop->getAnnotation("Column");
    if ($colAnn != null) {
        $colName := $colAnn->getString("name");
        if ($colName == "") {
            $colName = $prop->getName();  // 默认使用属性名
        }
        echo "Column: " + $colName;
    }
}
```

---

## 六、ORM 注解定义

### 6.1 核心注解（`sola.orm.annotation`）

```sola
namespace sola.orm.annotation

use sola.annotation.Attribute;
use sola.annotation.Target;
use sola.annotation.ElementType;
use sola.annotation.Inherited;

// 标记实体类
@Attribute
@Target([ElementType::CLASS])
@Inherited
public class Entity {
}

// 指定表名
@Attribute
@Target([ElementType::CLASS])
public class Table {
    public string $name;
    public string $schema;
    public string $charset;
    public string $engine;
    
    public function __construct(
        string $name,
        string $schema = "",
        string $charset = "utf8mb4",
        string $engine = "InnoDB"
    ) {
        $this->name = $name;
        $this->schema = $schema;
        $this->charset = $charset;
        $this->engine = $engine;
    }
}

// 字段映射
@Attribute
@Target([ElementType::PROPERTY])
public class Column {
    public string $name;
    public string $type;
    public int $length;
    public bool $nullable;
    public string $default;
    public string $comment;
    
    public function __construct(
        string $name = "",
        string $type = "",
        int $length = 0,
        bool $nullable = true,
        string $default = "",
        string $comment = ""
    ) {
        $this->name = $name;
        $this->type = $type;
        $this->length = $length;
        $this->nullable = $nullable;
        $this->default = $default;
        $this->comment = $comment;
    }
}

// 主键
@Attribute
@Target([ElementType::PROPERTY])
public class PrimaryKey {
    public bool $autoIncrement;
    
    public function __construct(bool $autoIncrement = true) {
        $this->autoIncrement = $autoIncrement;
    }
}

// 时间戳自动维护
@Attribute
@Target([ElementType::PROPERTY])
public class CreatedAt {
}

@Attribute
@Target([ElementType::PROPERTY])
public class UpdatedAt {
}

// 软删除
@Attribute
@Target([ElementType::PROPERTY])
public class SoftDelete {
}

// 忽略字段（不映射到数据库）
@Attribute
@Target([ElementType::PROPERTY])
public class Ignore {
}
```

### 6.2 索引注解

```sola
namespace sola.orm.annotation

use sola.annotation.Attribute;
use sola.annotation.Target;
use sola.annotation.ElementType;
use sola.annotation.Repeatable;

@Attribute
@Target([ElementType::CLASS])
@Repeatable(container = "sola.orm.annotation.Indexes")
public class Index {
    public string $name;
    public string[] $columns;
    public bool $unique;
    
    public function __construct(
        string[] $columns,
        string $name = "",
        bool $unique = false
    ) {
        $this->columns = $columns;
        $this->name = $name;
        $this->unique = $unique;
    }
}

@Attribute
@Target([ElementType::CLASS])
public class Indexes {
    public Index[] $value;
    
    public function __construct(Index[] $value) {
        $this->value = $value;
    }
}

// 唯一约束（属性级别）
@Attribute
@Target([ElementType::PROPERTY])
public class Unique {
    public string $name;
    
    public function __construct(string $name = "") {
        $this->name = $name;
    }
}
```

### 6.3 关联注解

```sola
namespace sola.orm.annotation

use sola.annotation.Attribute;
use sola.annotation.Target;
use sola.annotation.ElementType;

public enum FetchMode {
    EAGER,   // 立即加载
    LAZY     // 延迟加载
}

@Attribute
@Target([ElementType::PROPERTY])
public class HasOne {
    public string $target;
    public string $foreignKey;
    public FetchMode $fetch;
    
    public function __construct(
        string $target,
        string $foreignKey = "",
        FetchMode $fetch = FetchMode::LAZY
    ) {
        $this->target = $target;
        $this->foreignKey = $foreignKey;
        $this->fetch = $fetch;
    }
}

@Attribute
@Target([ElementType::PROPERTY])
public class HasMany {
    public string $target;
    public string $foreignKey;
    public FetchMode $fetch;
    
    public function __construct(
        string $target,
        string $foreignKey = "",
        FetchMode $fetch = FetchMode::LAZY
    ) {
        $this->target = $target;
        $this->foreignKey = $foreignKey;
        $this->fetch = $fetch;
    }
}

@Attribute
@Target([ElementType::PROPERTY])
public class BelongsTo {
    public string $target;
    public string $foreignKey;
    public string $ownerKey;
    public FetchMode $fetch;
    
    public function __construct(
        string $target,
        string $foreignKey = "",
        string $ownerKey = "id",
        FetchMode $fetch = FetchMode::EAGER
    ) {
        $this->target = $target;
        $this->foreignKey = $foreignKey;
        $this->ownerKey = $ownerKey;
        $this->fetch = $fetch;
    }
}

@Attribute
@Target([ElementType::PROPERTY])
public class BelongsToMany {
    public string $target;
    public string $pivot;
    public string $foreignPivotKey;
    public string $relatedPivotKey;
    public FetchMode $fetch;
    
    public function __construct(
        string $target,
        string $pivot = "",
        string $foreignPivotKey = "",
        string $relatedPivotKey = "",
        FetchMode $fetch = FetchMode::LAZY
    ) {
        $this->target = $target;
        $this->pivot = $pivot;
        $this->foreignPivotKey = $foreignPivotKey;
        $this->relatedPivotKey = $relatedPivotKey;
        $this->fetch = $fetch;
    }
}
```

### 6.4 使用示例

```sola
namespace app.models

use sola.orm.annotation.Entity;
use sola.orm.annotation.Table;
use sola.orm.annotation.Column;
use sola.orm.annotation.PrimaryKey;
use sola.orm.annotation.Index;
use sola.orm.annotation.Unique;
use sola.orm.annotation.CreatedAt;
use sola.orm.annotation.UpdatedAt;
use sola.orm.annotation.SoftDelete;
use sola.orm.annotation.HasMany;
use sola.orm.annotation.BelongsTo;
use sola.orm.Model;

@Entity
@Table("users", charset = "utf8mb4")
@Index(columns = ["email"], unique = true)
public class User extends Model {
    
    @PrimaryKey
    @Column("id", type = "bigint")
    public int $id;
    
    @Column("name", length = 100, nullable = false)
    public string $name;
    
    @Column(length = 255)
    @Unique
    public string $email;
    
    @Column(type = "text")
    public string $bio;
    
    @CreatedAt
    public string $createdAt;
    
    @UpdatedAt
    public string $updatedAt;
    
    @SoftDelete
    public string $deletedAt;
    
    @HasMany("Post", foreignKey = "user_id")
    public dynamic $posts;
    
    @BelongsTo("Department", foreignKey = "dept_id")
    public Department $department;
}
```

---

## 七、向后兼容

### 7.1 迁移策略

1. **第一阶段**：支持新语法，旧语法警告但不报错
2. **第二阶段**：旧语法报警告，建议迁移
3. **第三阶段**：移除旧语法支持

### 7.2 兼容模式

```sola
// 编译器选项
// sola.toml
[compiler]
annotation_mode = "strict"  // "strict" | "compatible"
```

- `strict`：注解必须先定义
- `compatible`：未定义的注解当作元数据（警告）

### 7.3 自动迁移工具

```bash
# 扫描旧注解，生成注解定义
sola migrate:annotations --scan src/

# 输出：
# Found 3 undefined annotations:
#   @Table(name) - used 5 times
#   @Column(name, type, nullable) - used 23 times  
#   @PrimaryKey - used 5 times
# 
# Generated: src/annotations/orm.sola
```

---

## 八、实现计划

### Phase 1：基础支持（优先）

- [ ] 解析器支持注解命名参数 `@Foo(name = "value")`
- [ ] 编译器识别 `@Attribute` 标记的类
- [ ] 基本的编译期验证（注解类是否存在）
- [ ] 字节码存储注解参数（命名参数格式）

### Phase 2：元注解

- [ ] 实现 `@Attribute` 元注解
- [ ] 实现 `@Target` - 限制使用位置
- [ ] 实现 `@Retention` - 保留策略
- [ ] 实现 `@Inherited` - 继承
- [ ] 实现 `@Repeatable` - 重复使用

### Phase 3：反射 API

- [ ] `sola.reflect.ClassReflector` 类
- [ ] `sola.reflect.PropertyReflector` 类
- [ ] `sola.reflect.MethodReflector` 类
- [ ] `sola.reflect.Annotation` 类

### Phase 4：ORM 注解

- [ ] 定义 `sola.orm.annotation` 包
- [ ] ORM 框架集成（使用反射读取注解）
- [ ] 文档和示例

---

## 九、参考

- [Java Annotations](https://docs.oracle.com/javase/tutorial/java/annotations/)
- [PHP 8 Attributes](https://www.php.net/manual/en/language.attributes.php)
- [Kotlin Annotations](https://kotlinlang.org/docs/annotations.html)
- [TypeScript Decorators](https://www.typescriptlang.org/docs/handbook/decorators.html)
