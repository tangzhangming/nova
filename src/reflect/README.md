# sola.reflect

Sola 反射 API 包，提供运行时类型信息和元数据访问能力。

## 概述

`sola.reflect` 包提供了一组类，用于在运行时检查和操作类、属性、方法和注解。

## 核心类

### ClassReflector

类反射器，用于访问类的元数据。

```sola
use sola.reflect.ClassReflector;

// 从类名创建
$reflector := ClassReflector::forClass("app.models.User");

// 从对象创建
$user := new User();
$reflector := ClassReflector::forObject($user);

// 获取类注解
$annotations := $reflector->getAnnotations();

// 检查是否有指定注解
if ($reflector->hasAnnotation("Entity")) {
    $entityAnn := $reflector->getAnnotation("Entity");
}

// 获取属性
$props := $reflector->getProperties();

// 获取方法
$methods := $reflector->getMethods();
```

### PropertyReflector

属性反射器，用于访问和操作属性。

```sola
$prop := $reflector->getProperty("name");

// 获取属性注解
$colAnn := $prop->getAnnotation("Column");
if ($colAnn != null) {
    $colName := $colAnn->getString("name");
    $nullable := $colAnn->getBool("nullable");
}

// 获取/设置值
$value := $prop->getValue($user);
$prop->setValue($user, "new value");
```

### MethodReflector

方法反射器，用于访问和调用方法。

```sola
$method := $reflector->getMethod("save");

// 获取方法注解
if ($method->hasAnnotation("Transaction")) {
    // 方法需要事务
}

// 调用方法
$result := $method->invoke($user, $arg1, $arg2);
```

### Annotation

注解对象，用于访问注解信息。

```sola
$ann := $reflector->getAnnotation("Table");

// 获取注解名
$name := $ann->getName();

// 获取参数
$tableName := $ann->getString("name");
$schema := $ann->getString("schema");
$nullable := $ann->getBool("nullable");
$length := $ann->getInt("length");

// 检查参数是否存在
if ($ann->has("charset")) {
    $charset := $ann->getString("charset");
}
```

## 完整示例

```sola
use sola.reflect.ClassReflector;

// 定义带注解的模型类
@Entity
@Table("users")
public class User {
    @Column(name = "id", primaryKey = true)
    public int $id;
    
    @Column(name = "user_name", nullable = false)
    public string $name;
    
    @Column
    public string $email;
}

// 使用反射读取注解
$reflector := ClassReflector::forClass("User");

// 获取表名
$tableAnn := $reflector->getAnnotation("Table");
if ($tableAnn != null) {
    $tableName := $tableAnn->getString("0"); // 位置参数
    echo "Table: " + $tableName;
}

// 遍历带 @Column 的属性
foreach ($reflector->getProperties() as $prop) {
    $colAnn := $prop->getAnnotation("Column");
    if ($colAnn != null) {
        $colName := $colAnn->getString("name");
        if ($colName == "") {
            $colName = $prop->getName(); // 默认使用属性名
        }
        echo "Column: " + $colName;
    }
}
```

## Native 函数

反射 API 依赖以下原生函数（由运行时提供）：

- `native_reflect_get_class($obj)` - 获取对象的类名
- `native_reflect_get_class_annotations($className)` - 获取类注解
- `native_reflect_get_property_annotations($className, $propName)` - 获取属性注解
- `native_reflect_get_method_annotations($className, $methodName)` - 获取方法注解
- `native_reflect_get_properties($className)` - 获取属性列表
- `native_reflect_get_methods($className)` - 获取方法列表
- `native_reflect_get_property($obj, $propName)` - 获取属性值
- `native_reflect_set_property($obj, $propName, $value)` - 设置属性值
- `native_reflect_invoke_method($obj, $methodName, $args)` - 调用方法
- `native_reflect_new_instance($className, $args)` - 创建实例
- `native_reflect_is_attribute($className)` - 检查是否是注解类
- `native_reflect_get_parent($className)` - 获取父类名
- `native_reflect_get_interfaces($className)` - 获取实现的接口
