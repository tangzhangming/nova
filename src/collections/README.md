# Sola Collections 标准库

完整的集合框架，设计对标 Java Collections Framework 和 C# System.Collections.Generic。

## 概览

### 类继承层次

```
IIterable<T>
    └── ICollection<T>
            ├── IList<T>
            │       ├── ArrayList<T>
            │       └── LinkedList<T>
            ├── ISet<T>
            │       ├── HashSet<T>
            │       ├── LinkedHashSet<T>
            │       └── TreeSet<T>
            ├── IQueue<T>
            │       ├── Queue<T>
            │       ├── PriorityQueue<T>
            │       └── IDeque<T>
            │               ├── ArrayDeque<T>
            │               └── LinkedList<T>
            └── IStack<T>
                    └── Stack<T>

IMap<K, V>
    ├── HashMap<K, V>
    ├── LinkedHashMap<K, V>
    └── TreeMap<K, V>
```

### 快速对照表

| 需求 | 推荐类 | 时间复杂度 |
|------|--------|-----------|
| 随机访问列表 | `ArrayList<T>` | 访问 O(1), 尾部增删 O(1) |
| 频繁插入删除 | `LinkedList<T>` | 头尾 O(1), 中间 O(n) |
| 快速查找集合 | `HashSet<T>` | O(1) 平均 |
| 有序集合 | `TreeSet<T>` | O(log n) |
| 保持插入顺序集合 | `LinkedHashSet<T>` | O(1) 平均 |
| 键值映射 | `HashMap<K, V>` | O(1) 平均 |
| 有序键值映射 | `TreeMap<K, V>` | O(log n) |
| 保持插入顺序映射 | `LinkedHashMap<K, V>` | O(1) 平均 |
| LRU 缓存 | `LinkedHashMap<K, V>(true)` | O(1) 平均 |
| FIFO 队列 | `Queue<T>` / `LinkedList<T>` | O(1) |
| LIFO 栈 | `Stack<T>` | O(1) |
| 优先队列 | `PriorityQueue<T>` | 入队/出队 O(log n) |
| 双端队列 | `ArrayDeque<T>` / `LinkedList<T>` | O(1) |

## 快速开始

```sola
use sola.collections.*;

// ArrayList - 动态数组
$list := new ArrayList<int>();
$list->add(1);
$list->add(2);
$list->add(3);
echo $list->get(0);  // 1

// HashSet - 哈希集合
$set := new HashSet<string>();
$set->add("apple");
$set->add("banana");
echo $set->contains("apple");  // true

// HashMap - 哈希映射
$map := new HashMap<string, int>();
$map->put("one", 1);
$map->put("two", 2);
echo $map->get("one");  // 1

// Stack - 栈
$stack := new Stack<int>();
$stack->push(1);
$stack->push(2);
echo $stack->pop();  // 2

// Queue - 队列
$queue := new Queue<string>();
$queue->enqueue("first");
$queue->enqueue("second");
echo $queue->dequeue();  // first

// PriorityQueue - 优先队列
$pq := new PriorityQueue<int>();
$pq->add(3);
$pq->add(1);
$pq->add(2);
echo $pq->poll();  // 1 (最小元素优先)
```

## 详细 API

### ArrayList\<T\>

动态数组，支持随机访问。

```sola
use sola.collections.ArrayList;

// 创建
$list := new ArrayList<int>();
$list := ArrayList::of(1, 2, 3);
$list := ArrayList::fromArray([1, 2, 3]);

// 基本操作
$list->add(4);              // 添加到末尾
$list->insert(0, 0);        // 在索引0插入
$list->get(0);              // 获取索引0的元素
$list->set(0, 10);          // 设置索引0的元素
$list->removeAt(0);         // 移除索引0的元素
$list->remove(10);          // 移除第一个值为10的元素

// 查询
$list->size();              // 元素数量
$list->isEmpty();           // 是否为空
$list->contains(5);         // 是否包含
$list->indexOf(5);          // 首次出现的索引
$list->lastIndexOf(5);      // 最后出现的索引

// 子列表
$sub := $list->subList(1, 3);  // [1, 3) 区间

// 排序和反转
$list->sort();              // 自然排序
$list->sort($comparator);   // 自定义排序
$list->reverse();           // 反转

// 函数式操作
$list->filter(fn($x) => $x > 5);     // 过滤
$list->map(fn($x) => $x * 2);        // 映射
$list->reduce(0, fn($a, $b) => $a + $b);  // 归约
$list->find(fn($x) => $x > 5);       // 查找
$list->any(fn($x) => $x > 5);        // 任意满足
$list->all(fn($x) => $x > 0);        // 全部满足
$list->forEach(fn($x) => echo $x);   // 遍历

// 转换
$arr := $list->toArray();
$str := $list->join(", ");
```

### LinkedList\<T\>

双向链表，同时实现 IList 和 IDeque 接口。

```sola
use sola.collections.LinkedList;

$list := new LinkedList<string>();

// 列表操作
$list->add("middle");
$list->addFirst("first");
$list->addLast("last");

// 双端队列操作
echo $list->getFirst();   // first
echo $list->getLast();    // last
$list->removeFirst();
$list->removeLast();

// 栈操作
$list->push("top");
echo $list->pop();        // top

// 队列操作
$list->offer("item");
echo $list->poll();       // item
```

### HashSet\<T\>

基于哈希表的集合，不保证顺序。

```sola
use sola.collections.HashSet;

$set := new HashSet<int>();
$set := HashSet::of(1, 2, 3);

// 基本操作
$set->add(4);
$set->remove(1);
$set->contains(2);

// 集合运算
$a := HashSet::of(1, 2, 3);
$b := HashSet::of(2, 3, 4);

$union := $a->union($b);                    // {1, 2, 3, 4}
$intersection := $a->intersection($b);      // {2, 3}
$difference := $a->difference($b);          // {1}
$symDiff := $a->symmetricDifference($b);   // {1, 4}

// 集合关系
$a->isSubsetOf($b);        // false
$a->isSupersetOf($b);      // false
$a->overlaps($b);          // true
$a->setEquals($b);         // false
```

### TreeSet\<T\>

有序集合，元素按自然顺序或自定义比较器排序。

```sola
use sola.collections.TreeSet;

$set := new TreeSet<int>();
$set->add(3);
$set->add(1);
$set->add(2);

// 导航方法
echo $set->first();        // 1 (最小)
echo $set->last();         // 3 (最大)
echo $set->lower(2);       // 1 (小于2的最大值)
echo $set->higher(2);      // 3 (大于2的最小值)
echo $set->floor(2);       // 2 (小于等于2的最大值)
echo $set->ceiling(2);     // 2 (大于等于2的最小值)

// 范围视图
$sub := $set->subSet(1, 3);    // [1, 3)
$head := $set->headSet(2);      // < 2
$tail := $set->tailSet(2);      // >= 2

// 移除极值
$set->pollFirst();         // 移除并返回最小值
$set->pollLast();          // 移除并返回最大值

// 逆序遍历
$iter := $set->descendingIterator();
while ($iter->hasNext()) {
    echo $iter->next();
}
```

### HashMap\<K, V\>

基于哈希表的键值映射。

```sola
use sola.collections.HashMap;

$map := new HashMap<string, int>();

// 基本操作
$map->put("a", 1);
$map->put("b", 2);
echo $map->get("a");              // 1
echo $map->getOrDefault("c", 0);  // 0

// 条件操作
$map->putIfAbsent("a", 10);       // 不会覆盖
$map->replace("a", 100);          // 替换
$map->replaceIf("a", 100, 1);     // 条件替换

// 移除
$map->remove("a");
$map->removeIf("b", 2);           // 条件移除

// 视图
$keys := $map->keys();            // ISet<K>
$vals := $map->values();          // ICollection<V>
$entries := $map->entries();      // ISet<MapEntry<K, V>>

// 遍历
$map->forEach(function($k, $v) {
    echo $k + ": " + $v;
});

// 高级操作
$map->compute("a", fn($k, $v) => $v + 1);
$map->computeIfAbsent("c", fn($k) => 0);
$map->merge($other, fn($old, $new) => $old + $new);
```

### TreeMap\<K, V\>

有序键值映射，键按自然顺序或自定义比较器排序。

```sola
use sola.collections.TreeMap;

$map := new TreeMap<int, string>();
$map->put(3, "three");
$map->put(1, "one");
$map->put(2, "two");

// 导航方法
echo $map->firstKey();        // 1
echo $map->lastKey();         // 3
echo $map->lowerKey(2);       // 1
echo $map->higherKey(2);      // 3

// 条目操作
$entry := $map->firstEntry();
echo $entry->getKey() + ": " + $entry->getValue();

$map->pollFirstEntry();       // 移除并返回第一个条目
$map->pollLastEntry();        // 移除并返回最后一个条目

// 范围视图
$sub := $map->subMap(1, 3);   // 键在 [1, 3) 范围
$head := $map->headMap(2);    // 键 < 2
$tail := $map->tailMap(2);    // 键 >= 2
```

### LinkedHashMap\<K, V\>

保持插入顺序（或访问顺序）的哈希映射。

```sola
use sola.collections.LinkedHashMap;

// 插入顺序（默认）
$map := new LinkedHashMap<string, int>();
$map->put("c", 3);
$map->put("a", 1);
$map->put("b", 2);

$map->forEach(function($k, $v) {
    echo $k;  // c, a, b (插入顺序)
});

// 访问顺序（LRU缓存）
$lru := new LinkedHashMap<string, int>(true);
$lru->put("a", 1);
$lru->put("b", 2);
$lru->put("c", 3);
$lru->get("a");  // 访问 a

$lru->forEach(function($k, $v) {
    echo $k;  // b, c, a (a 最近访问，移到末尾)
});
```

### Stack\<T\>

后进先出（LIFO）栈。

```sola
use sola.collections.Stack;

$stack := new Stack<int>();

$stack->push(1);
$stack->push(2);
$stack->push(3);

echo $stack->peek();       // 3 (查看栈顶)
echo $stack->pop();        // 3 (弹出)
echo $stack->search(1);    // 2 (从栈顶开始的位置，1-based)

// 安全操作
$val := $stack->tryPop();   // 空时返回 null
$val := $stack->tryPeek();  // 空时返回 null
```

### Queue\<T\>

先进先出（FIFO）队列。

```sola
use sola.collections.Queue;

$queue := new Queue<string>();

$queue->enqueue("first");
$queue->enqueue("second");
$queue->enqueue("third");

echo $queue->front();      // first (查看队首)
echo $queue->dequeue();    // first (出队)

// 安全操作
$queue->offer("item");     // 入队（容量受限时返回 false）
$val := $queue->poll();    // 空时返回 null
$val := $queue->peek();    // 空时返回 null
```

### ArrayDeque\<T\>

双端队列，支持在两端高效操作。

```sola
use sola.collections.ArrayDeque;

$deque := new ArrayDeque<int>();

// 两端操作
$deque->addFirst(1);
$deque->addLast(2);
echo $deque->getFirst();   // 1
echo $deque->getLast();    // 2
$deque->removeFirst();
$deque->removeLast();

// 作为栈使用
$deque->push(1);
echo $deque->pop();

// 作为队列使用
$deque->offer(1);
echo $deque->poll();
```

### PriorityQueue\<T\>

基于二叉堆的优先队列。

```sola
use sola.collections.PriorityQueue;

// 最小堆（默认）
$minHeap := new PriorityQueue<int>();
$minHeap->add(3);
$minHeap->add(1);
$minHeap->add(2);
echo $minHeap->poll();  // 1
echo $minHeap->poll();  // 2
echo $minHeap->poll();  // 3

// 最大堆
$maxHeap := PriorityQueue::maxHeap<int>();
$maxHeap->add(3);
$maxHeap->add(1);
$maxHeap->add(2);
echo $maxHeap->poll();  // 3

// 自定义比较器
$pq := new PriorityQueue<Task>($taskComparator);

// 转为排序数组
$sorted := $pq->toSortedArray();
```

### Collections 工具类

静态工具方法集合。

```sola
use sola.collections.{ArrayList, Collections};

$list := ArrayList::of(3, 1, 4, 1, 5, 9);

// 排序
Collections::sort($list);
Collections::sort($list, $comparator);

// 反转和洗牌
Collections::reverse($list);
Collections::shuffle($list);

// 极值
echo Collections::max($list);  // 9
echo Collections::min($list);  // 1

// 二分查找（需先排序）
Collections::sort($list);
$index := Collections::binarySearch($list, 4);

// 填充和替换
Collections::fill($list, 0);
Collections::replaceAll($list, 0, 1);

// 旋转
Collections::rotate($list, 2);  // 向右旋转2位

// 统计
echo Collections::frequency($list, 1);  // 1出现的次数
echo Collections::sum($list);           // 求和
echo Collections::average($list);       // 平均值

// 检查
$disjoint := Collections::disjoint($list1, $list2);  // 是否无交集

// 创建
$list := Collections::nCopies(5, "x");        // ["x", "x", "x", "x", "x"]
$list := Collections::singletonList("x");     // ["x"]
$set := Collections::singleton("x");          // {"x"}
$map := Collections::singletonMap("k", "v");  // {"k": "v"}

// 连接
$combined := Collections::concat($list1, $list2, $list3);
```

## 自定义比较器

```sola
use sola.collections.{IComparator, ArrayList, TreeSet};

// 实现比较器接口
class PersonAgeComparator implements IComparator<Person> {
    public function compare(Person $a, Person $b): int {
        return $a->age - $b->age;
    }
}

// 使用比较器
$list := new ArrayList<Person>();
$list->sort(new PersonAgeComparator());

$set := new TreeSet<Person>(new PersonAgeComparator());
```

## 迭代器

```sola
use sola.collections.ArrayList;

$list := ArrayList::of(1, 2, 3);

// 使用 foreach
foreach ($list as $item) {
    echo $item;
}

// 使用迭代器
$iter := $list->iterator();
while ($iter->hasNext()) {
    $item := $iter->next();
    echo $item;
    
    // 迭代时移除
    if ($item == 2) {
        $iter->remove();
    }
}
```

## 异常处理

```sola
use sola.collections.*;

try {
    $list := new ArrayList<int>();
    $list->get(0);  // IndexOutOfBoundsException
    
    $set := new HashSet<int>();
    $set->iterator()->next();  // NoSuchElementException
    
} catch (IndexOutOfBoundsException $e) {
    echo "索引越界: " + $e->getMessage();
} catch (NoSuchElementException $e) {
    echo "没有更多元素: " + $e->getMessage();
} catch (ConcurrentModificationException $e) {
    echo "并发修改: " + $e->getMessage();
}
```

## 与 Java/C# 对照

| Java | C# | Sola |
|------|-----|------|
| `ArrayList<T>` | `List<T>` | `ArrayList<T>` |
| `LinkedList<T>` | `LinkedList<T>` | `LinkedList<T>` |
| `HashSet<T>` | `HashSet<T>` | `HashSet<T>` |
| `TreeSet<T>` | `SortedSet<T>` | `TreeSet<T>` |
| `LinkedHashSet<T>` | - | `LinkedHashSet<T>` |
| `HashMap<K,V>` | `Dictionary<K,V>` | `HashMap<K,V>` |
| `TreeMap<K,V>` | `SortedDictionary<K,V>` | `TreeMap<K,V>` |
| `LinkedHashMap<K,V>` | - | `LinkedHashMap<K,V>` |
| `Stack<T>` | `Stack<T>` | `Stack<T>` |
| `Queue<T>` | `Queue<T>` | `Queue<T>` |
| `ArrayDeque<T>` | - | `ArrayDeque<T>` |
| `PriorityQueue<T>` | `PriorityQueue<T>` | `PriorityQueue<T>` |
| `Collections` | - | `Collections` |

## 性能提示

1. **选择正确的数据结构**
   - 需要随机访问？用 `ArrayList`
   - 频繁头尾操作？用 `LinkedList` 或 `ArrayDeque`
   - 需要去重？用 `HashSet` 或 `TreeSet`
   - 需要快速查找？用 `HashMap` 或 `HashSet`

2. **避免在迭代时修改集合**
   - 使用迭代器的 `remove()` 方法
   - 或者先收集要删除的元素，再批量删除

3. **有序 vs 无序**
   - 不需要排序时，`Hash*` 类比 `Tree*` 类更快
   - 需要排序时，一次性添加后用 `Collections::sort()` 可能比维护 `Tree*` 更快

4. **预分配容量**
   - 如果知道大致元素数量，可以预分配（ArrayList 有 `ensureCapacity`）

5. **使用正确的集合操作**
   - 批量操作（`addAll`, `removeAll`）通常比循环调用单个操作更高效







