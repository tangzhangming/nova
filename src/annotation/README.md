# sola.annotation

Sola 注解系统的元注解（Meta-Annotations）定义包。

## 概述

元注解用于约束和描述用户定义的注解类。所有注解类必须使用 `@Attribute` 元注解标记。

## 元注解列表

### @Attribute

标记一个类为注解类。只有标记了 `@Attribute` 的类才能作为注解使用。

```sola
@Attribute
public class Entity {
}
```

### @Target

限制注解可以使用的位置。

```sola
@Attribute
@Target([ElementType::PROPERTY])
public class Column {
    // Column 只能用于属性
}
```

可用的 `ElementType` 值：
- `CLASS` - 类
- `INTERFACE` - 接口
- `METHOD` - 方法
- `PROPERTY` - 属性
- `PARAMETER` - 方法参数
- `CONSTRUCTOR` - 构造函数
- `ALL` - 所有位置（默认）

### @Retention

指定注解的保留策略。

```sola
@Attribute
@Retention(RetentionPolicy::RUNTIME)
public class Entity {
}
```

可用的 `RetentionPolicy` 值：
- `SOURCE` - 仅源码，编译后丢弃
- `COMPILE` - 编译期可用，运行时不可见
- `RUNTIME` - 运行时可用（默认）

### @Inherited

标记注解可以被子类继承。

```sola
@Attribute
@Inherited
public class Entity {
}

@Entity
public class BaseModel { }

// User 自动继承 @Entity
public class User extends BaseModel { }
```

### @Repeatable

允许注解在同一元素上重复使用。

```sola
@Attribute
@Repeatable(container = "Indexes")
public class Index {
    public function __construct(public string $name, public string[] $columns) {}
}

@Index(name = "idx_name", columns = ["name"])
@Index(name = "idx_email", columns = ["email"])
public class User { }
```

## 使用示例

```sola
use sola.annotation.Attribute;
use sola.annotation.Target;
use sola.annotation.ElementType;

@Attribute
@Target([ElementType::CLASS])
public class Entity {
}

@Attribute
@Target([ElementType::PROPERTY])
public class Column {
    public string $name;
    public bool $nullable;
    
    public function __construct(
        string $name = "",
        bool $nullable = true
    ) {
        $this->name = $name;
        $this->nullable = $nullable;
    }
}

// 使用自定义注解
@Entity
public class User {
    @Column(name = "user_id", nullable = false)
    public int $id;
    
    @Column
    public string $name;
}
```
