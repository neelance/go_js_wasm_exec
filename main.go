package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"syscall"
	"time"

	"github.com/perlin-network/life/exec"
)

var Undefined = &struct{}{}

func main() {
	input, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	memory := &ArrayBuffer{}

	goObj := map[string]interface{}{
		"_makeFuncWrapper": func(this *Object, args []interface{}) interface{} {
			return &FuncWrapper{id: args[0]}
		},
		"_pendingEvent": nil,
	}

	vm := &VM{}
	vm.values = []interface{}{
		math.NaN(),
		float64(0),
		nil,
		true,
		false,
		&Object{Props: map[string]interface{}{
			"Object": &Object{
				New: func(args []interface{}) interface{} {
					panic("new Object")
				},
			},
			"Array": &Object{
				New: func(args []interface{}) interface{} {
					panic("new Array")
				},
			},
			"Int8Array":    typedArrayClass,
			"Int16Array":   typedArrayClass,
			"Int32Array":   typedArrayClass,
			"Uint8Array":   typedArrayClass,
			"Uint16Array":  typedArrayClass,
			"Uint32Array":  typedArrayClass,
			"Float32Array": typedArrayClass,
			"Float64Array": typedArrayClass,
			"process":      &Object{},
			"fs": &Object{Props: map[string]interface{}{
				"constants": &Object{Props: map[string]interface{}{
					"O_WRONLY": syscall.O_WRONLY,
					"O_RDWR":   syscall.O_RDWR,
					"O_CREAT":  syscall.O_CREAT,
					"O_TRUNC":  syscall.O_TRUNC,
					"O_APPEND": syscall.O_APPEND,
					"O_EXCL":   syscall.O_EXCL,
				}},
				"write": func(this *Object, args []interface{}) interface{} {
					fd := int(args[0].(float64))
					buffer := args[1].(*TypedArray)
					offset := int(args[2].(float64))
					length := int(args[3].(float64))
					b := buffer.contents()[offset : offset+length]
					callback := args[5].(*FuncWrapper)

					if args[4] != nil {
						position := int64(args[4].(float64))
						syscall.Pwrite(fd, b, position)
					} else {
						syscall.Write(fd, b)
					}

					goObj["_pendingEvent"] = &Object{Props: map[string]interface{}{
						"id":   callback.id,
						"this": nil,
						"args": &[]interface{}{
							nil,
							length,
						},
					}}
					return nil
				},
			}},
		}}, // global
		&Object{Props: map[string]interface{}{
			"buffer": memory,
		}}, // memory
		&Object{Props: goObj}, // go
	}
	vm.refs = make(map[interface{}]int)

	vm.VirtualMachine, err = exec.NewVirtualMachine(input, exec.VMConfig{}, vm, nil)
	if err != nil {
		panic(err)
	}
	memory.data = vm.Memory

	run, ok := vm.GetFunctionExport("run")
	if !ok {
		panic("function not found: run")
	}

	resume, ok := vm.GetFunctionExport("resume")
	if !ok {
		panic("function not found: resume")
	}

	if _, err := vm.Run(run, 0, 0); err != nil {
		panic(err)
	}

	for !vm.exited {
		if _, err := vm.Run(resume); err != nil {
			panic(err)
		}
	}

	os.Exit(vm.exitCode)
}

type VM struct {
	*exec.VirtualMachine
	exited   bool
	exitCode int
	values   []interface{}
	refs     map[interface{}]int
}

func (vm *VM) ResolveGlobal(module, field string) int64 {
	panic("no global imports")
}

func (vm *VM) ResolveFunc(module, field string) exec.FunctionImport {
	if module != "go" {
		panic("module not found: " + module)
	}

	f, ok := imports[field]
	if !ok {
		panic("function not found: " + field)
	}

	return func(_ *exec.VirtualMachine) int64 {
		sp := int(uint32(vm.GetCurrentFrame().Locals[0]))
		f(sp, vm)
		return 0
	}
}

var imports = map[string]func(sp int, vm *VM){
	"runtime.wasmExit":              wasmExit,
	"runtime.wasmWrite":             wasmWrite,
	"runtime.nanotime":              nanotime,
	"runtime.walltime":              walltime,
	"runtime.scheduleTimeoutEvent":  scheduleTimeoutEvent,
	"runtime.clearTimeoutEvent":     clearTimeoutEvent,
	"runtime.getRandomData":         getRandomData,
	"syscall/js.stringVal":          stringVal,
	"syscall/js.valueGet":           valueGet,
	"syscall/js.valueSet":           valueSet,
	"syscall/js.valueIndex":         valueIndex,
	"syscall/js.valueSetIndex":      valueSetIndex,
	"syscall/js.valueCall":          valueCall,
	"syscall/js.valueInvoke":        valueInvoke,
	"syscall/js.valueNew":           valueNew,
	"syscall/js.valueLength":        valueLength,
	"syscall/js.valuePrepareString": valuePrepareString,
	"syscall/js.valueLoadString":    valueLoadString,
	"syscall/js.valueInstanceOf":    valueInstanceOf,
	"debug":                         debug,
}

// func wasmExit(code int32)
func wasmExit(sp int, vm *VM) {
	vm.exited = true
	vm.exitCode = int(getInt32(sp+8, vm))
}

// func wasmWrite(fd uintptr, p unsafe.Pointer, n int32)
func wasmWrite(sp int, vm *VM) {
	fd := int(getInt64(sp+8, vm))
	p := int(getInt64(sp+16, vm))
	n := int(getInt32(sp+24, vm))
	syscall.Write(fd, vm.Memory[p:p+n])
}

// func nanotime() int64
func nanotime(sp int, vm *VM) {
	setInt64(sp+8, time.Now().UnixNano(), vm)
}

// func walltime() (sec int64, nsec int32)
func walltime(sp int, vm *VM) {
	panic("not implemented: walltime") // TODO
}

// func scheduleTimeoutEvent(delay int64) int32
func scheduleTimeoutEvent(sp int, vm *VM) {
	panic("not implemented: scheduleTimeoutEvent") // TODO
}

// func clearTimeoutEvent(id int32)
func clearTimeoutEvent(sp int, vm *VM) {
	panic("not implemented: clearTimeoutEvent") // TODO
}

// func getRandomData(r []byte)
func getRandomData(sp int, vm *VM) {
	if _, err := rand.Read(loadSlice(sp+8, vm)); err != nil {
		panic(err)
	}
}

// func stringVal(value string) ref
func stringVal(sp int, vm *VM) {
	panic("not implemented: stringVal") // TODO
}

// func valueGet(v ref, p string) ref
func valueGet(sp int, vm *VM) {
	name := loadString(sp+16, vm)
	result, ok := loadValue(sp+8, vm).(*Object).Props[name]
	// TODO getsp
	if !ok {
		panic("missing property: " + name) // TODO
	}
	storeValue(sp+32, result, vm)
}

// func valueSet(v ref, p string, x ref)
func valueSet(sp int, vm *VM) {
	loadValue(sp+8, vm).(*Object).Props[loadString(sp+16, vm)] = loadValue(sp+32, vm)
}

// func valueIndex(v ref, i int) ref
func valueIndex(sp int, vm *VM) {
	result := (*loadValue(sp+8, vm).(*[]interface{}))[getInt64(sp+16, vm)]
	storeValue(sp+24, result, vm)
}

// valueSetIndex(v ref, i int, x ref)
func valueSetIndex(sp int, vm *VM) {
	panic("not implemented: valueSetIndex") // TODO
}

// func valueCall(v ref, m string, args []ref) (ref, bool)
func valueCall(sp int, vm *VM) {
	// TODO error handling
	v := loadValue(sp+8, vm).(*Object)
	name := loadString(sp+16, vm)
	m, ok := v.Props[name]
	if !ok {
		panic("missing method: " + name) // TODO
	}
	args := loadSliceOfValues(sp+32, vm)
	result := m.(func(*Object, []interface{}) interface{})(v, args)
	// TODO getsp
	storeValue(sp+56, result, vm)
	setUint8(sp+64, 1, vm)
}

// func valueInvoke(v ref, args []ref) (ref, bool)
func valueInvoke(sp int, vm *VM) {
	panic("not implemented: valueInvoke") // TODO
}

// func valueNew(v ref, args []ref) (ref, bool)
func valueNew(sp int, vm *VM) {
	// TODO error handling
	v := loadValue(sp+8, vm)
	args := loadSliceOfValues(sp+16, vm)
	result := v.(*Object).New(args)
	// TODO getsp
	storeValue(sp+40, result, vm)
	setUint8(sp+48, 1, vm)
}

// func valueLength(v ref) int
func valueLength(sp int, vm *VM) {
	array := loadValue(sp+8, vm).(*[]interface{})
	setInt64(sp+16, int64(len(*array)), vm)
}

// valuePrepareString(v ref) (ref, int)
func valuePrepareString(sp int, vm *VM) {
	panic("not implemented: valuePrepareString") // TODO
}

// valueLoadString(v ref, b []byte)
func valueLoadString(sp int, vm *VM) {
	panic("not implemented: valueLoadString") // TODO
}

// func valueInstanceOf(v ref, t ref) bool
func valueInstanceOf(sp int, vm *VM) {
	panic("not implemented: valueInstanceOf") // TODO
}

func debug(sp int, vm *VM) {
	fmt.Println("DEBUG:", sp)
}

func getUint32(addr int, vm *VM) uint32 {
	return binary.LittleEndian.Uint32(vm.Memory[addr:])
}

func getInt32(addr int, vm *VM) int32 {
	return int32(getUint32(addr, vm))
}

func getUint64(addr int, vm *VM) uint64 {
	return binary.LittleEndian.Uint64(vm.Memory[addr:])
}

func getInt64(addr int, vm *VM) int64 {
	return int64(getUint64(addr, vm))
}

func getFloat64(addr int, vm *VM) float64 {
	return math.Float64frombits(getUint64(addr, vm))
}

func setUint8(addr int, v uint8, vm *VM) {
	vm.Memory[addr] = v
}

func setUint32(addr int, v uint32, vm *VM) {
	binary.LittleEndian.PutUint32(vm.Memory[addr:], v)
}

func setInt32(addr int, v int32, vm *VM) {
	setUint32(addr, uint32(v), vm)
}

func setUint64(addr int, v uint64, vm *VM) {
	binary.LittleEndian.PutUint64(vm.Memory[addr:], v)
}

func setInt64(addr int, v int64, vm *VM) {
	setUint64(addr, uint64(v), vm)
}

func setFloat64(addr int, v float64, vm *VM) {
	setUint64(addr, math.Float64bits(v), vm)
}

func loadString(addr int, vm *VM) string {
	saddr := getInt64(addr+0, vm)
	len := getInt64(addr+8, vm)
	return string(vm.Memory[saddr : saddr+len])
}

func loadValue(addr int, vm *VM) interface{} {
	f := getFloat64(addr, vm)
	if f == 0 {
		return Undefined
	}
	if !math.IsNaN(f) {
		return f
	}

	id := getUint32(addr, vm)
	return vm.values[id]
}

func storeValue(addr int, v interface{}, vm *VM) {
	const nanHead = 0x7FF80000

	if i, ok := v.(int); ok {
		v = float64(i)
	}
	if i, ok := v.(uint); ok {
		v = float64(i)
	}
	if v, ok := v.(float64); ok {
		if math.IsNaN(v) {
			setUint32(addr+4, nanHead, vm)
			setUint32(addr, 0, vm)
			return
		}
		if v == 0 {
			setUint32(addr+4, nanHead, vm)
			setUint32(addr, 1, vm)
			return
		}
		setFloat64(addr, v, vm)
		return
	}

	switch v {
	case Undefined:
		setFloat64(addr, 0, vm)
		return
	case nil:
		setUint32(addr+4, nanHead, vm)
		setUint32(addr, 2, vm)
		return
	case true:
		setUint32(addr+4, nanHead, vm)
		setUint32(addr, 3, vm)
		return
	case false:
		setUint32(addr+4, nanHead, vm)
		setUint32(addr, 4, vm)
		return
	}

	ref, ok := vm.refs[v]
	if !ok {
		ref = len(vm.values)
		vm.values = append(vm.values, v)
		vm.refs[v] = ref
	}

	typeFlag := 0
	switch v.(type) {
	case string:
		typeFlag = 1
		// TODO symbol
		// TODO function
	}
	setUint32(addr+4, uint32(nanHead|typeFlag), vm)
	setUint32(addr, uint32(ref), vm)
}

func loadSlice(addr int, vm *VM) []byte {
	array := int(getInt64(addr+0, vm))
	len := int(getInt64(addr+8, vm))
	return vm.Memory[array : array+len]
}

func loadSliceOfValues(addr int, vm *VM) []interface{} {
	array := int(getInt64(addr+0, vm))
	len := int(getInt64(addr+8, vm))
	a := make([]interface{}, len)
	for i := range a {
		a[i] = loadValue(array+i*8, vm)
	}
	return a
}

type Object struct {
	Props map[string]interface{}
	New   func(args []interface{}) interface{}
}

type ArrayBuffer struct {
	data []byte
}

type TypedArray struct {
	Buffer *ArrayBuffer
	Offset int
	Length int
}

func (a *TypedArray) contents() []byte {
	return a.Buffer.data[a.Offset : a.Offset+a.Length]
}

var typedArrayClass = &Object{
	New: func(args []interface{}) interface{} {
		return &TypedArray{
			Buffer: args[0].(*ArrayBuffer),
			Offset: int(args[1].(float64)),
			Length: int(args[2].(float64)),
		}
	},
}

type FuncWrapper struct {
	id interface{}
}
