# Sola 泛型编程指南

> **版本**: Sola 1.0  
> **创建日期**: 2026-01-06  
> **最后更新**: 2026-01-06

Sola 支持泛型编程，允许你编写类型安全且可复用的代码。本文档介绍泛型的语法和使用方法。

## 实现状态

Sola 的泛型系统已完整实现以下功能：

- ✅ 泛型类和接口定义
- ✅ 多类型参数支持
- ✅ 泛型方法
- ✅ `extends` 约束（编译时验证）
- ✅ `implements` 约束（编译时验证）
- ✅ `where` 子句多重约束
- ✅ 类型推断（构造函数和方法调用）
- ✅ 类型擦除（运行时）

所有约束验证在编译时进行，确保类型安全。

---

## 目录

1. [泛型类](#泛型类)
2. [泛型接口](#泛型接口)
3. [泛型方法](#泛型方法)
4. [类型约束](#类型约束)
5. [泛型类型实例化](#泛型类型实例化)
6. [类型推断](#类型推断)
7. [最佳实践](#最佳实践)
8. [常见错误](#常见错误)

---

## 泛型类

### 基本语法

```sola
public class Box<T> {
    private T $value;
    
    public function __construct(T $value) {
        $this->value = $value;
    }
    
    public function get(): T {
        return $this->value;
    }
    
    public function set(T $value): void {
        $this->value = $value;
    }
}
```

### 多类型参数

```sola
public class Pair<K, V> {
    private K $key;
    private V $value;
    
    public function __construct(K $key, V $value) {
        $this->key = $key;
        $this->value = $value;
    }
    
    public function getKey(): K {
        return $this->key;
    }
    
    public function getValue(): V {
        return $this->value;
    }
}
```

### 使用泛型类

```sola
// 显式指定类型参数
Box<int> $intBox = new Box<int>(42);
int $val = $intBox->get();

Box<string> $strBox = new Box<string>("hello");
string $str = $strBox->get();

// 键值对
Pair<string, int> $pair = new Pair<string, int>("age", 25);
echo $pair->getKey();   // "age"
echo $pair->getValue(); // 25
```

---

## 泛型接口

### 基本语法

```sola
public interface Container<T> {
    public function add(T $item): void;
    public function get(int $index): T;
    public function size(): int;
}
```

### 实现泛型接口

```sola
public class ArrayList<T> implements Container<T> {
    private T[] $items = [];
    
    public function add(T $item): void {
        $this->items[] = $item;
    }
    
    public function get(int $index): T {
        return $this->items[$index];
    }
    
    public function size(): int {
        return len($this->items);
    }
}
```

### 泛型接口继承

```sola
public interface Comparable<T> {
    public function compareTo(T $other): int;
}

public interface Sortable<T> extends Comparable<T> {
    public function sort(): void;
}
```

---

## 泛型方法

### 类中的泛型方法

```sola
public class Utils {
    // 泛型静态方法
    public static function swap<T>(T[] $arr, int $i, int $j): void {
        T $temp = $arr[$i];
        $arr[$i] = $arr[$j];
        $arr[$j] = $temp;
    }
    
    // 泛型实例方法
    public function transform<T, R>(T $input, func(T): R $fn): R {
        return $fn($input);
    }
}
```

### 使用泛型方法

```sola
int[] $nums = [1, 2, 3, 4, 5];

// 显式指定类型参数
Utils::swap<int>($nums, 0, 4);

// 类型推断（编译器自动推断类型）
Utils::swap($nums, 0, 4);
```

---

## 类型约束

### extends 约束

使用 `extends` 限制类型参数必须是某个类的子类：

```sola
public class NumberBox<T extends Number> {
    private T $value;
    
    public function __construct(T $value) {
        $this->value = $value;
    }
    
    public function doubleValue(): float {
        return $this->value->toFloat() * 2;
    }
}

// 正确：int 是 Number 的子类
NumberBox<int> $box = new NumberBox<int>(10);

// 错误：string 不是 Number 的子类
// NumberBox<string> $box = new NumberBox<string>("hello"); // 编译错误
```

### implements 约束

使用 `implements` 限制类型参数必须实现某个或多个接口：

```sola
public class SortedList<T implements Comparable<T>> {
    private T[] $items = [];
    
    public function add(T $item): void {
        $this->items[] = $item;
        $this->sortItems();
    }
    
    private function sortItems(): void {
        // 可以安全调用 compareTo，因为 T 实现了 Comparable
        // ...
    }
}
```

**多个接口约束**：可以在类型参数中同时指定多个接口：

```sola
public class Processor<T implements IComparable, ISerializable> {
    // T 必须同时实现 IComparable 和 ISerializable
    public function process(T $item): void {
        $item->compareTo($item);  // 安全：T 实现了 IComparable
        $json = $item->serialize(); // 安全：T 实现了 ISerializable
    }
}
```

**组合约束**：可以同时使用 `extends` 和 `implements`：

```sola
public class Repository<T extends Entity implements ISerializable> {
    // T 必须继承 Entity 并实现 ISerializable
    public function save(T $entity): void {
        $entity->getId();        // 安全：T 继承 Entity
        $json = $entity->serialize(); // 安全：T 实现 ISerializable
    }
}
```

### where 子句（多重约束）

对于复杂约束，可以使用 `where` 子句在类声明后添加额外的约束：

```sola
public class Repository<T> where T extends Entity, T implements Serializable {
    public function save(T $entity): void {
        // T 同时满足继承 Entity 和实现 Serializable
        string $json = $entity->serialize();
        Database::insert($entity->getTable(), $json);
    }
}
```

**where 子句语法**：

```sola
public class MyClass<T, K> 
    where T extends BaseClass, 
          T implements IInterface1, IInterface2,
          K extends AnotherClass {
    // 类体
}
```

**使用场景**：
- 当约束过于复杂，不适合放在类型参数声明中时
- 需要为多个类型参数分别指定不同约束时
- 需要更清晰的约束表达时

**注意**：`where` 子句中的约束会与类型参数声明中的约束合并，编译器会验证所有约束。

---

## 泛型类型实例化

### 变量声明

```sola
// 显式指定类型
Box<int> $intBox = new Box<int>(42);
Pair<string, int> $pair = new Pair<string, int>("key", 100);

// 类型推断（简写）
$intBox := new Box<int>(42);
$pair := new Pair<string, int>("key", 100);
```

### 作为参数类型

```sola
function processBox(Box<int> $box): int {
    return $box->get() * 2;
}

function swapPair<K, V>(Pair<K, V> $pair): Pair<V, K> {
    return new Pair<V, K>($pair->getValue(), $pair->getKey());
}
```

### 作为返回类型

```sola
function createIntBox(int $value): Box<int> {
    return new Box<int>($value);
}

function createPair<K, V>(K $key, V $value): Pair<K, V> {
    return new Pair<K, V>($key, $value);
}
```

---

## 类型推断

Sola 编译器可以在许多情况下自动推断类型参数，减少代码冗余：

### 构造函数类型推断

编译器可以从构造函数参数自动推断泛型类型参数：

```sola
// 从构造函数参数推断类型
$box := new Box(42);        // 推断为 Box<int>
$box := new Box("hello");   // 推断为 Box<string>

$pair := new Pair("key", 100);  // 推断为 Pair<string, int>
```

**推断规则**：
- 如果构造函数有参数，编译器会从第一个参数的类型推断第一个类型参数
- 如果类型参数数量多于参数数量，剩余的类型参数需要显式指定
- 如果无法推断，编译器会报错要求显式指定

**显式指定仍然有效**：

```sola
// 显式指定类型参数（推荐用于复杂场景）
Box<int> $box = new Box<int>(42);
```

### 方法调用类型推断

泛型方法的类型参数可以从实参类型推断：

```sola
int[] $nums = [1, 2, 3];
Utils::swap($nums, 0, 2);  // 推断 T 为 int

string[] $strs = ["a", "b", "c"];
Utils::swap($strs, 0, 2);  // 推断 T 为 string
```

**推断规则**：
- 从方法参数的类型推断对应的类型参数
- 如果参数类型不明确（如 `any`），需要显式指定类型参数

### 无法推断时必须显式指定

以下情况需要显式指定类型参数：

```sola
// 1. 工厂方法无法从参数推断返回类型
Box<int> $box = Box::empty<int>();

// 2. 方法没有参数或参数类型不明确
Utils::identity<string>("hello");

// 3. 类型参数数量多于可推断的参数
Pair<string, int> $pair = Utils::makePair("key", 100);
```

### 类型推断的限制

- 类型推断仅在编译时进行
- 如果推断失败，编译器会要求显式指定类型参数
- 复杂嵌套泛型类型建议显式指定以提高可读性

---

## 最佳实践

### 1. 使用有意义的类型参数名

```sola
// 好：使用描述性名称
public class Map<Key, Value> { ... }
public class Transformer<Input, Output> { ... }

// 不好：单字母不够清晰（但简单泛型可接受）
public class Map<K, V> { ... }  // 简单场景可接受
```

### 2. 优先使用接口约束

```sola
// 好：使用接口约束，更灵活
public class Sorter<T implements Comparable<T>> { ... }

// 不太好：使用具体类约束，限制了灵活性
public class Sorter<T extends SortableBase> { ... }
```

### 3. 避免过度泛型化

```sola
// 好：简单明了
public function sum(int[] $nums): int { ... }

// 过度：不必要的泛型化
public function sum<T extends Number>(T[] $nums): T { ... }
```

### 4. 使用类型别名简化复杂泛型

```sola
// 复杂泛型类型
Map<string, List<Pair<int, User>>> $data;

// 使用类型别名（未来版本支持）
// type UserDataMap = Map<string, List<Pair<int, User>>>;
// UserDataMap $data;
```

---

## 常见错误

### 1. 类型参数数量不匹配

```sola
// 错误：Box 需要 1 个类型参数
Box $box = new Box(42);  // 编译错误

// 正确
Box<int> $box = new Box<int>(42);
```

### 2. 类型约束不满足

```sola
// NumberBox 要求 T extends Number
NumberBox<string> $box;  // 编译错误：string 不满足 extends Number 约束

// 正确
NumberBox<int> $box;
NumberBox<float> $box;
```

**implements 约束违规**：

```sola
public class SortedList<T implements Comparable<T>> { ... }

// 错误：User 类没有实现 Comparable 接口
SortedList<User> $list;  // 编译错误

// 正确：ComparableUser 实现了 Comparable 接口
SortedList<ComparableUser> $list;
```

**多重约束违规**：

```sola
public class Repository<T> 
    where T extends Entity, T implements Serializable { ... }

// 错误：User 实现了 Serializable 但没有继承 Entity
Repository<User> $repo;  // 编译错误

// 正确：EntityUser 同时满足两个约束
Repository<EntityUser> $repo;
```

### 3. 重复的类型参数名

```sola
// 错误：T 重复定义
public class Bad<T, T> { ... }  // 编译错误

// 正确
public class Good<T, U> { ... }
```

### 4. 在运行时无法获取泛型类型信息

由于 Sola 使用类型擦除，运行时无法获取具体的类型参数：

```sola
Box<int> $box = new Box<int>(42);
// typeof($box) 返回 "Box"，而不是 "Box<int>"
```

---

## 类型擦除说明

Sola 使用**类型擦除**实现泛型，这意味着：

1. 泛型类型信息仅在编译时存在
2. 运行时所有泛型类型被擦除为其边界类型或 `any`
3. 类型安全在编译时保证，运行时不进行额外检查

这种设计的优点：
- 向后兼容，不影响现有字节码
- 运行时开销为零
- 与非泛型代码无缝互操作

缺点：
- 无法在运行时获取类型参数信息
- 无法创建泛型数组 `new T[10]`（需使用工厂模式）

---

## 完整示例

```sola
// 定义一个泛型栈
public class Stack<T> {
    private T[] $items = [];
    
    public function push(T $item): void {
        $this->items[] = $item;
    }
    
    public function pop(): T {
        if ($this->isEmpty()) {
            throw new Exception("Stack is empty");
        }
        return array_pop($this->items);
    }
    
    public function peek(): T {
        if ($this->isEmpty()) {
            throw new Exception("Stack is empty");
        }
        return $this->items[len($this->items) - 1];
    }
    
    public function isEmpty(): bool {
        return len($this->items) == 0;
    }
    
    public function size(): int {
        return len($this->items);
    }
}

// 使用泛型栈
Stack<int> $intStack = new Stack<int>();
$intStack->push(1);
$intStack->push(2);
$intStack->push(3);
echo $intStack->pop();  // 3Stack<string> $strStack = new Stack<string>();
$strStack->push("hello");
$strStack->push("world");
echo $strStack->pop();  // "world"
```