package desc

import (
	"fmt"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/internal"
)

// LoadFileDescriptor creates a file descriptor using the bytes returned by
// proto.FileDescriptor. Descriptors are cached so that they do not need to be
// re-processed if the same file is fetched again later.
func LoadFileDescriptor(file string) (*FileDescriptor, error) {
	d, err := protoregistry.GlobalFiles.FindFileByPath(file)
	if err != nil {
		return nil, err
	}
	if fd := cache.get(d); fd != nil {
		return fd.(*FileDescriptor), nil
	}

	var fd *FileDescriptor
	cache.withLock(func(c descriptorCache) {
		fd, err = toFileDescriptor(d, c)
	})
	return fd, err
}

func toFileDescriptor(d protoreflect.FileDescriptor, c descriptorCache) (*FileDescriptor, error) {
	f := c.get(d)
	if f != nil {
		return f.(*FileDescriptor), nil
	}

	fdp := protodesc.ToFileDescriptorProto(d)
	return wrapFile(d, fdp, c)
}

// LoadMessageDescriptor loads descriptor using the encoded descriptor proto returned by
// Message.Descriptor() for the given message type. If the given type is not recognized,
// then a nil descriptor is returned.
func LoadMessageDescriptor(message string) (*MessageDescriptor, error) {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(message))
	if err != nil {
		if err == protoregistry.NotFound {
			return nil, nil
		}
		return nil, err
	}
	md := mt.Descriptor()
	d := cache.get(md)
	if d != nil {
		return d.(*MessageDescriptor), nil
	}

	cache.withLock(func(cache descriptorCache) {
		d, err = wrapMessage(md, cache)
	})
	return d, err
}

// LoadMessageDescriptorForType loads descriptor using the encoded descriptor proto returned
// by message.Descriptor() for the given message type. If the given type is not recognized,
// then a nil descriptor is returned.
func LoadMessageDescriptorForType(messageType reflect.Type) (*MessageDescriptor, error) {
	m, err := messageFromType(messageType)
	if err != nil {
		return nil, err
	}
	return LoadMessageDescriptorForMessage(m)
}

// LoadMessageDescriptorForMessage loads descriptor using the encoded descriptor proto
// returned by message.Descriptor(). If the given type is not recognized, then a nil
// descriptor is returned.
func LoadMessageDescriptorForMessage(message proto.Message) (*MessageDescriptor, error) {
	// efficiently handle dynamic messages
	type descriptorable interface {
		GetMessageDescriptor() *MessageDescriptor
	}
	if d, ok := message.(descriptorable); ok {
		return d.GetMessageDescriptor(), nil
	}

	var md protoreflect.MessageDescriptor
	if m, ok := message.(protoreflect.ProtoMessage); ok {
		md = m.ProtoReflect().Descriptor()
	} else {

	}
	name := proto.MessageName(message)
	if name == "" {
		return nil, nil
	}
	m := getMessageFromCache(name)
	if m != nil {
		return m, nil
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()
	return loadMessageDescriptorForTypeLocked(name, message.(protoMessage))
}

func messageFromType(mt reflect.Type) (proto.Message, error) {
	if mt.Kind() != reflect.Ptr {
		mt = reflect.PtrTo(mt)
	}
	m, ok := reflect.Zero(mt).Interface().(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to create message from type: %v", mt)
	}
	return m, nil
}

func loadMessageDescriptorForTypeLocked(name string, message protoMessage) (*MessageDescriptor, error) {
	m := messagesCache[name]
	if m != nil {
		return m, nil
	}

	fdb, _ := message.Descriptor()
	fd, err := internal.DecodeFileDescriptor(name, fdb)
	if err != nil {
		return nil, err
	}

	f, err := toFileDescriptorLocked(fd)
	if err != nil {
		return nil, err
	}
	putCacheLocked(fd.GetName(), f)
	return f.FindSymbol(name).(*MessageDescriptor), nil
}

// interface implemented by all generated enums
type protoEnum interface {
	EnumDescriptor() ([]byte, []int)
}

// NB: There is no LoadEnumDescriptor that takes a fully-qualified enum name because
// it is not useful since protoc-gen-go does not expose the name anywhere in generated
// code or register it in a way that is it accessible for reflection code. This also
// means we have to cache enum descriptors differently -- we can only cache them as
// they are requested, as opposed to caching all enum types whenever a file descriptor
// is cached. This is because we need to know the generated type of the enums, and we
// don't know that at the time of caching file descriptors.

// LoadEnumDescriptorForType loads descriptor using the encoded descriptor proto returned
// by enum.EnumDescriptor() for the given enum type.
func LoadEnumDescriptorForType(enumType reflect.Type) (*EnumDescriptor, error) {
	// we cache descriptors using non-pointer type
	if enumType.Kind() == reflect.Ptr {
		enumType = enumType.Elem()
	}
	e := getEnumFromCache(enumType)
	if e != nil {
		return e, nil
	}
	enum, err := enumFromType(enumType)
	if err != nil {
		return nil, err
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()
	return loadEnumDescriptorForTypeLocked(enumType, enum)
}

// LoadEnumDescriptorForEnum loads descriptor using the encoded descriptor proto
// returned by enum.EnumDescriptor().
func LoadEnumDescriptorForEnum(enum protoEnum) (*EnumDescriptor, error) {
	et := reflect.TypeOf(enum)
	// we cache descriptors using non-pointer type
	if et.Kind() == reflect.Ptr {
		et = et.Elem()
		enum = reflect.Zero(et).Interface().(protoEnum)
	}
	e := getEnumFromCache(et)
	if e != nil {
		return e, nil
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()
	return loadEnumDescriptorForTypeLocked(et, enum)
}

func enumFromType(et reflect.Type) (protoEnum, error) {
	e, ok := reflect.Zero(et).Interface().(protoEnum)
	if !ok {
		if et.Kind() != reflect.Ptr {
			et = et.Elem()
		}
		e, ok = reflect.Zero(et).Interface().(protoEnum)
	}
	if !ok {
		return nil, fmt.Errorf("failed to create enum from type: %v", et)
	}
	return e, nil
}

func loadEnumDescriptorForTypeLocked(et reflect.Type, enum protoEnum) (*EnumDescriptor, error) {
	e := enumCache[et]
	if e != nil {
		return e, nil
	}

	fdb, path := enum.EnumDescriptor()
	name := fmt.Sprintf("%v", et)
	fd, err := internal.DecodeFileDescriptor(name, fdb)
	if err != nil {
		return nil, err
	}
	// see if we already have cached "rich" descriptor
	f, ok := filesCache[fd.GetName()]
	if !ok {
		f, err = toFileDescriptorLocked(fd)
		if err != nil {
			return nil, err
		}
		putCacheLocked(fd.GetName(), f)
	}

	ed := findEnum(f, path)
	enumCache[et] = ed
	return ed, nil
}

func findEnum(fd *FileDescriptor, path []int) *EnumDescriptor {
	if len(path) == 1 {
		return fd.GetEnumTypes()[path[0]]
	}
	md := fd.GetMessageTypes()[path[0]]
	for _, i := range path[1 : len(path)-1] {
		md = md.GetNestedMessageTypes()[i]
	}
	return md.GetNestedEnumTypes()[path[len(path)-1]]
}

// LoadFieldDescriptorForExtension loads the field descriptor that corresponds to the given
// extension description.
func LoadFieldDescriptorForExtension(ext *proto.ExtensionDesc) (*FieldDescriptor, error) {
	file, err := LoadFileDescriptor(ext.Filename)
	if err != nil {
		return nil, err
	}
	field, ok := file.FindSymbol(ext.Name).(*FieldDescriptor)
	// make sure descriptor agrees with attributes of the ExtensionDesc
	if !ok || !field.IsExtension() || field.GetOwner().GetFullyQualifiedName() != proto.MessageName(ext.ExtendedType) ||
		field.GetNumber() != ext.Field {
		return nil, fmt.Errorf("file descriptor contained unexpected object with name %s", ext.Name)
	}
	return field, nil
}
