package bytecode

// ============================================================================
// 接口虚表 (VTable) 实现
// ============================================================================

// VTable 接口虚表
// 用于将接口方法调用从 O(n) 优化到 O(1)
type VTable struct {
	InterfaceName string        // 接口名称
	ClassName     string        // 实现类的名称
	Methods       []VTableEntry // 方法表（按接口方法索引排序）
}

// VTableEntry 虚表条目
type VTableEntry struct {
	MethodIndex int     // 在接口中的方法索引
	MethodName  string  // 接口方法名
	ImplMethod  *Method // 实际实现的方法
}

// Lookup 根据方法索引查找实现方法（O(1) 查找）
func (vt *VTable) Lookup(methodIndex int) *Method {
	if methodIndex < 0 || methodIndex >= len(vt.Methods) {
		return nil
	}
	return vt.Methods[methodIndex].ImplMethod
}

// GetMethodIndex 根据方法名获取在接口中的索引
func (vt *VTable) GetMethodIndex(methodName string) int {
	for i, entry := range vt.Methods {
		if entry.MethodName == methodName {
			return i
		}
	}
	return -1
}

// GenerateVTable 为给定的类-接口对生成 VTable
// class: 实现类
// iface: 接口类
func GenerateVTable(class *Class, iface *Class) *VTable {
	if !iface.IsInterface {
		return nil // iface 必须是接口
	}

	vtable := &VTable{
		InterfaceName: iface.Name,
		ClassName:     class.Name,
		Methods:       make([]VTableEntry, 0),
	}

	// 获取接口的所有方法（包括继承的接口）
	interfaceMethods := collectInterfaceMethods(iface)

	// 为每个接口方法找到实现
	for i, ifaceMethod := range interfaceMethods {
		// 在类中查找实现方法（支持方法重载）
		implMethod := findMethodImplementation(class, ifaceMethod.Name, ifaceMethod.Arity)

		if implMethod == nil {
			// 如果没有找到实现，这应该是编译时错误
			// 但这里我们返回一个包含 nil 的条目，运行时可以检测
			vtable.Methods = append(vtable.Methods, VTableEntry{
				MethodIndex: i,
				MethodName:  ifaceMethod.Name,
				ImplMethod:  nil,
			})
		} else {
			vtable.Methods = append(vtable.Methods, VTableEntry{
				MethodIndex: i,
				MethodName:  ifaceMethod.Name,
				ImplMethod:  implMethod,
			})
		}
	}

	return vtable
}

// collectInterfaceMethods 收集接口的所有方法（包括继承的接口）
// 返回方法列表，保持顺序（按接口定义顺序）
func collectInterfaceMethods(iface *Class) []*Method {
	result := make([]*Method, 0)
	seen := make(map[string]bool) // 避免重复

	// 收集继承接口的方法
	if len(iface.Implements) > 0 {
		// 注意：这里我们无法访问所有类，所以需要在 BuildAllVTables 中传递 classes map
		// 这里先只收集当前接口的方法
	}

	// 收集当前接口的方法
	for name, methods := range iface.Methods {
		if len(methods) > 0 && !seen[name] {
			result = append(result, methods[0]) // 接口方法不支持重载，取第一个
			seen[name] = true
		}
	}

	return result
}

// findMethodImplementation 在类中查找方法实现
func findMethodImplementation(class *Class, methodName string, arity int) *Method {
	// 遍历类及其父类
	for c := class; c != nil; c = c.Parent {
		if methods, ok := c.Methods[methodName]; ok {
			// 尝试找到匹配参数数量的方法
			for _, m := range methods {
				if m.Arity == arity {
					return m
				}
			}
			// 如果没有精确匹配，返回第一个（可能有默认参数）
			if len(methods) > 0 {
				return methods[0]
			}
		}
	}
	return nil
}

// BuildAllVTables 为所有类构建 VTable
// classes: 所有类的映射（类名 -> 类）
func BuildAllVTables(classes map[string]*Class) map[string]map[string]*VTable {
	result := make(map[string]map[string]*VTable)

	// 收集所有接口
	interfaces := make(map[string]*Class)
	for name, class := range classes {
		if class.IsInterface {
			interfaces[name] = class
		}
	}

	// 为每个实现了接口的类构建 VTable
	for className, class := range classes {
		if class.IsInterface {
			continue // 跳过接口本身
		}

		// 为类实现的每个接口构建 VTable
		for _, ifaceName := range class.Implements {
			if iface, ok := interfaces[ifaceName]; ok {
				vtable := GenerateVTableWithInterfaces(class, iface, classes)
				if vtable != nil {
					if result[className] == nil {
						result[className] = make(map[string]*VTable)
					}
					result[className][ifaceName] = vtable
					// 同时存储在类的 VTables 字段中
					class.VTables[ifaceName] = vtable
				}
			}
		}
	}

	return result
}

// GenerateVTableWithInterfaces 生成 VTable（支持接口继承）
func GenerateVTableWithInterfaces(class *Class, iface *Class, allClasses map[string]*Class) *VTable {
	if !iface.IsInterface {
		return nil
	}

	vtable := &VTable{
		InterfaceName: iface.Name,
		ClassName:     class.Name,
		Methods:       make([]VTableEntry, 0),
	}

	// 收集接口的所有方法（包括继承的接口）
	interfaceMethods := collectInterfaceMethodsRecursive(iface, allClasses)

	// 为每个接口方法找到实现
	for i, ifaceMethod := range interfaceMethods {
		implMethod := findMethodImplementation(class, ifaceMethod.Name, ifaceMethod.Arity)

		vtable.Methods = append(vtable.Methods, VTableEntry{
			MethodIndex: i,
			MethodName:  ifaceMethod.Name,
			ImplMethod:  implMethod,
		})
	}

	return vtable
}

// collectInterfaceMethodsRecursive 递归收集接口的所有方法（包括继承的接口）
func collectInterfaceMethodsRecursive(iface *Class, allClasses map[string]*Class) []*Method {
	result := make([]*Method, 0)
	seen := make(map[string]bool)

	// 递归收集继承接口的方法
	for _, parentIfaceName := range iface.Implements {
		if parentIface, ok := allClasses[parentIfaceName]; ok && parentIface.IsInterface {
			parentMethods := collectInterfaceMethodsRecursive(parentIface, allClasses)
			for _, m := range parentMethods {
				if !seen[m.Name] {
					result = append(result, m)
					seen[m.Name] = true
				}
			}
		}
	}

	// 收集当前接口的方法
	for name, methods := range iface.Methods {
		if len(methods) > 0 && !seen[name] {
			result = append(result, methods[0])
			seen[name] = true
		}
	}

	return result
}

// GetVTable 获取类的指定接口的 VTable
func (c *Class) GetVTable(interfaceName string) *VTable {
	return c.VTables[interfaceName]
}

