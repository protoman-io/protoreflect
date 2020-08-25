package desc

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// DescriptorWrapper wraps a protoreflect.Descriptor. All of the Descriptor
// implementations in this package implement this interface. This can be
// used to recover the underlying descriptor. Each descriptor type in this
// package also provides a strongly-typed form of this method, such as the
// following method for *FileDescriptor:
//    UnwrapFile() protoreflect.FileDescriptor
type DescriptorWrapper interface {
	Unwrap() protoreflect.Descriptor
}

func WrapDescriptor(d protoreflect.Descriptor) (Descriptor, error) {
	switch d := d.(type) {
	case protoreflect.FileDescriptor:
		return WrapFile(d)
	case protoreflect.MessageDescriptor:
		return WrapMessage(d)
	case protoreflect.FieldDescriptor:
		return WrapField(d)
	case protoreflect.EnumDescriptor:
		return WrapEnum(d)
	case protoreflect.EnumValueDescriptor:
		return WrapEnumValue(d)
	case protoreflect.ServiceDescriptor:
		return WrapService(d)
	case protoreflect.MethodDescriptor:
		return WrapMethod(d)
	default:
		return nil, fmt.Errorf("unknown descriptor type: %T", d)
	}
}

func WrapFile(d protoreflect.FileDescriptor) (*FileDescriptor, error) {
	return toFileDescriptor(d, noopCache{})
}

func WrapMessage(d protoreflect.MessageDescriptor) (*MessageDescriptor, error) {
	parent, err := WrapDescriptor(d.Parent())
	if err != nil {
		return nil, err
	}
	switch p := parent.(type) {
	case *FileDescriptor:
		return p.messages[d.Index()], nil
	case *MessageDescriptor:
		return p.nested[d.Index()], nil
	default:
		return nil, fmt.Errorf("message has unexpected parent type: %T", parent)
	}
}

func WrapField(d protoreflect.FieldDescriptor) (*FieldDescriptor, error) {
	parent, err := WrapDescriptor(d.Parent())
	if err != nil {
		return nil, err
	}
	switch p := parent.(type) {
	case *FileDescriptor:
		return p.extensions[d.Index()], nil
	case *MessageDescriptor:
		if d.IsExtension() {
			return p.extensions[d.Index()], nil
		}
		return p.fields[d.Index()], nil
	default:
		return nil, fmt.Errorf("field has unexpected parent type: %T", parent)
	}
}

func WrapOneOf(d protoreflect.OneofDescriptor) (*OneOfDescriptor, error) {
	parent, err := WrapDescriptor(d.Parent())
	if err != nil {
		return nil, err
	}
	if p, ok := parent.(*MessageDescriptor); ok {
		return p.oneOfs[d.Index()], nil
	}
	return nil, fmt.Errorf("oneof has unexpected parent type: %T", parent)
}

func WrapEnum(d protoreflect.EnumDescriptor) (*EnumDescriptor, error) {
	parent, err := WrapDescriptor(d.Parent())
	if err != nil {
		return nil, err
	}
	switch p := parent.(type) {
	case *FileDescriptor:
		return p.enums[d.Index()], nil
	case *MessageDescriptor:
		return p.enums[d.Index()], nil
	default:
		return nil, fmt.Errorf("enum has unexpected parent type: %T", parent)
	}
}

func WrapEnumValue(d protoreflect.EnumValueDescriptor) (*EnumValueDescriptor, error) {
	parent, err := WrapDescriptor(d.Parent())
	if err != nil {
		return nil, err
	}
	if p, ok := parent.(*EnumDescriptor); ok {
		return p.values[d.Index()], nil
	}
	return nil, fmt.Errorf("enum value has unexpected parent type: %T", parent)
}

func WrapService(d protoreflect.ServiceDescriptor) (*ServiceDescriptor, error) {
	parent, err := WrapDescriptor(d.Parent())
	if err != nil {
		return nil, err
	}
	if p, ok := parent.(*FileDescriptor); ok {
		return p.services[d.Index()], nil
	}
	return nil, fmt.Errorf("service has unexpected parent type: %T", parent)
}

func WrapMethod(d protoreflect.MethodDescriptor) (*MethodDescriptor, error) {
	parent, err := WrapDescriptor(d.Parent())
	if err != nil {
		return nil, err
	}
	if p, ok := parent.(*ServiceDescriptor); ok {
		return p.methods[d.Index()], nil
	}
	return nil, fmt.Errorf("method has unexpected parent type: %T", parent)
}
