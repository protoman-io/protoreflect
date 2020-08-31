package protoparse

import (
	"bytes"
	"fmt"
	"math"

	"github.com/golang/protobuf/proto"
	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/internal"
	"github.com/jhump/protoreflect/dynamic"
)

func (l *linker) interpretFileOptions(r *parseResult) error {
	fd := r.fd
	prefix := fd.GetPackage()
	if prefix != "" {
		prefix += "."
	}
	opts := fd.GetOptions()
	if opts != nil {
		if len(opts.UninterpretedOption) > 0 {
			if remain, err := l.interpretOptions(r, fd.GetName(), fd, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, md := range fd.GetMessageType() {
		fqn := prefix + md.GetName()
		if err := l.interpretMessageOptions(r, fqn, md); err != nil {
			return err
		}
	}
	for _, fld := range fd.GetExtension() {
		fqn := prefix + fld.GetName()
		if err := l.interpretFieldOptions(r, fqn, fld); err != nil {
			return err
		}
	}
	for _, ed := range fd.GetEnumType() {
		fqn := prefix + ed.GetName()
		if err := l.interpretEnumOptions(r, fqn, ed); err != nil {
			return err
		}
	}
	for _, sd := range fd.GetService() {
		fqn := prefix + sd.GetName()
		opts := sd.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := l.interpretOptions(r, fqn, sd, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
		for _, mtd := range sd.GetMethod() {
			mtdFqn := fqn + "." + mtd.GetName()
			opts := mtd.GetOptions()
			if len(opts.GetUninterpretedOption()) > 0 {
				if remain, err := l.interpretOptions(r, mtdFqn, mtd, opts, opts.UninterpretedOption); err != nil {
					return err
				} else {
					opts.UninterpretedOption = remain
				}
			}
		}
	}
	return nil
}

func (l *linker) interpretMessageOptions(r *parseResult, fqn string, md *dpb.DescriptorProto) error {
	opts := md.GetOptions()
	if opts != nil {
		if len(opts.UninterpretedOption) > 0 {
			if remain, err := l.interpretOptions(r, fqn, md, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, fld := range md.GetField() {
		fldFqn := fqn + "." + fld.GetName()
		if err := l.interpretFieldOptions(r, fldFqn, fld); err != nil {
			return err
		}
	}
	for _, ood := range md.GetOneofDecl() {
		oodFqn := fqn + "." + ood.GetName()
		opts := ood.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := l.interpretOptions(r, oodFqn, ood, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, fld := range md.GetExtension() {
		fldFqn := fqn + "." + fld.GetName()
		if err := l.interpretFieldOptions(r, fldFqn, fld); err != nil {
			return err
		}
	}
	for _, er := range md.GetExtensionRange() {
		erFqn := fmt.Sprintf("%s.%d-%d", fqn, er.GetStart(), er.GetEnd())
		opts := er.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := l.interpretOptions(r, erFqn, er, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, nmd := range md.GetNestedType() {
		nmdFqn := fqn + "." + nmd.GetName()
		if err := l.interpretMessageOptions(r, nmdFqn, nmd); err != nil {
			return err
		}
	}
	for _, ed := range md.GetEnumType() {
		edFqn := fqn + "." + ed.GetName()
		if err := l.interpretEnumOptions(r, edFqn, ed); err != nil {
			return err
		}
	}
	return nil
}

func (l *linker) interpretFieldOptions(r *parseResult, fqn string, fld *dpb.FieldDescriptorProto) error {
	opts := fld.GetOptions()
	if len(opts.GetUninterpretedOption()) > 0 {
		uo := opts.UninterpretedOption
		scope := fmt.Sprintf("field %s", fqn)

		// process json_name pseudo-option
		if index, err := findOption(r, scope, uo, "json_name"); err != nil && !r.lenient {
			return err
		} else if index >= 0 {
			opt := uo[index]
			optNode := r.getOptionNode(opt)

			// attribute source code info
			if on, ok := optNode.(*optionNode); ok {
				r.interpretedOptions[on] = []int32{-1, internal.Field_jsonNameTag}
			}
			uo = removeOption(uo, index)
			if opt.StringValue == nil {
				if err := r.errs.handleErrorWithPos(optNode.getValue().start(), "%s: expecting string value for json_name option", scope); err != nil {
					return err
				}
			} else {
				fld.JsonName = proto.String(string(opt.StringValue))
			}
		}

		// and process default pseudo-option
		if index, err := l.processDefaultOption(r, scope, fqn, fld, uo); err != nil && !r.lenient {
			return err
		} else if index >= 0 {
			// attribute source code info
			optNode := r.getOptionNode(uo[index])
			if on, ok := optNode.(*optionNode); ok {
				r.interpretedOptions[on] = []int32{-1, internal.Field_defaultTag}
			}
			uo = removeOption(uo, index)
		}

		if len(uo) == 0 {
			// no real options, only pseudo-options above? clear out options
			fld.Options = nil
		} else if remain, err := l.interpretOptions(r, fqn, fld, opts, uo); err != nil {
			return err
		} else {
			opts.UninterpretedOption = remain
		}
	}
	return nil
}

func (l *linker) processDefaultOption(res *parseResult, scope string, fqn string, fld *dpb.FieldDescriptorProto, uos []*dpb.UninterpretedOption) (defaultIndex int, err error) {
	found, err := findOption(res, scope, uos, "default")
	if err != nil || found == -1 {
		return -1, err
	}
	opt := uos[found]
	optNode := res.getOptionNode(opt)
	if fld.GetLabel() == dpb.FieldDescriptorProto_LABEL_REPEATED {
		return -1, res.errs.handleErrorWithPos(optNode.getName().start(), "%s: default value cannot be set because field is repeated", scope)
	}
	if fld.GetType() == dpb.FieldDescriptorProto_TYPE_GROUP || fld.GetType() == dpb.FieldDescriptorProto_TYPE_MESSAGE {
		return -1, res.errs.handleErrorWithPos(optNode.getName().start(), "%s: default value cannot be set because field is a message", scope)
	}
	val := optNode.getValue()
	if _, ok := val.(*aggregateLiteralNode); ok {
		return -1, res.errs.handleErrorWithPos(val.start(), "%s: default value cannot be an aggregate", scope)
	}
	mc := &messageContext{
		res:         res,
		file:        res.fd,
		elementName: fqn,
		elementType: descriptorType(fld),
		option:      opt,
	}
	v, err := l.fieldValue(res, mc, fld, val, true)
	if err != nil {
		return -1, res.errs.handleError(err)
	}
	if str, ok := v.(string); ok {
		fld.DefaultValue = proto.String(str)
	} else if b, ok := v.([]byte); ok {
		fld.DefaultValue = proto.String(encodeDefaultBytes(b))
	} else {
		var flt float64
		var ok bool
		if flt, ok = v.(float64); !ok {
			var flt32 float32
			if flt32, ok = v.(float32); ok {
				flt = float64(flt32)
			}
		}
		if ok {
			if math.IsInf(flt, 1) {
				fld.DefaultValue = proto.String("inf")
			} else if ok && math.IsInf(flt, -1) {
				fld.DefaultValue = proto.String("-inf")
			} else if ok && math.IsNaN(flt) {
				fld.DefaultValue = proto.String("nan")
			} else {
				fld.DefaultValue = proto.String(fmt.Sprintf("%v", v))
			}
		} else {
			fld.DefaultValue = proto.String(fmt.Sprintf("%v", v))
		}
	}
	return found, nil
}

func encodeDefaultBytes(b []byte) string {
	var buf bytes.Buffer
	writeEscapedBytes(&buf, b)
	return buf.String()
}

func (l *linker) interpretEnumOptions(r *parseResult, fqn string, ed *dpb.EnumDescriptorProto) error {
	opts := ed.GetOptions()
	if opts != nil {
		if len(opts.UninterpretedOption) > 0 {
			if remain, err := l.interpretOptions(r, fqn, ed, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	for _, evd := range ed.GetValue() {
		evdFqn := fqn + "." + evd.GetName()
		opts := evd.GetOptions()
		if len(opts.GetUninterpretedOption()) > 0 {
			if remain, err := l.interpretOptions(r, evdFqn, evd, opts, opts.UninterpretedOption); err != nil {
				return err
			} else {
				opts.UninterpretedOption = remain
			}
		}
	}
	return nil
}

func (l *linker) interpretOptions(res *parseResult, fqn string, element proto.Message, opts proto.Message, uninterpreted []*dpb.UninterpretedOption) ([]*dpb.UninterpretedOption, error) {
	optsd, err := desc.LoadMessageDescriptorForMessage(opts)
	if err != nil {
		if res.lenient {
			return uninterpreted, nil
		}
		return nil, res.errs.handleError(err)
	}
	dm := dynamic.NewMessage(optsd)
	err = dm.ConvertFrom(opts)
	if err != nil {
		if res.lenient {
			return uninterpreted, nil
		}
		node := res.nodes[element]
		return nil, res.errs.handleError(ErrorWithSourcePos{Pos: node.start(), Underlying: err})
	}

	mc := &messageContext{res: res, file: res.fd, elementName: fqn, elementType: descriptorType(element)}
	var remain []*dpb.UninterpretedOption
	for _, uo := range uninterpreted {
		node := res.getOptionNode(uo)
		if !uo.Name[0].GetIsExtension() && uo.Name[0].GetNamePart() == "uninterpreted_option" {
			if res.lenient {
				remain = append(remain, uo)
				continue
			}
			// uninterpreted_option might be found reflectively, but is not actually valid for use
			if err := res.errs.handleErrorWithPos(node.getName().start(), "%vinvalid option 'uninterpreted_option'", mc); err != nil {
				return nil, err
			}
		}
		mc.option = uo
		path, err := l.interpretField(res, mc, element, dm, uo, 0, nil)
		if err != nil {
			if res.lenient {
				remain = append(remain, uo)
				continue
			}
			return nil, err
		}
		if optn, ok := node.(*optionNode); ok {
			res.interpretedOptions[optn] = path
		}
	}

	if res.lenient {
		// If we're lenient, then we don't want to clobber the passed in message
		// and leave it partially populated. So we convert into a copy first
		optsClone := proto.Clone(opts)
		if err := dm.ConvertToDeterministic(optsClone); err != nil {
			// TODO: do this in a more granular way, so we can convert individual
			// fields and leave bad ones uninterpreted instead of skipping all of
			// the work we've done so far.
			return uninterpreted, nil
		}
		// conversion from dynamic message above worked, so now
		// it is safe to overwrite the passed in message
		opts.Reset()
		proto.Merge(opts, optsClone)

		return remain, nil
	}

	if err := dm.ValidateRecursive(); err != nil {
		node := res.nodes[element]
		if err := res.errs.handleErrorWithPos(node.start(), "error in %s options: %v", descriptorType(element), err); err != nil {
			return nil, err
		}
	}

	// nw try to convert into the passed in message and fail if not successful
	if err := dm.ConvertToDeterministic(opts); err != nil {
		node := res.nodes[element]
		return nil, res.errs.handleError(ErrorWithSourcePos{Pos: node.start(), Underlying: err})
	}

	return nil, nil
}

func (l *linker) interpretField(res *parseResult, mc *messageContext, element proto.Message, dm *dynamic.Message, opt *dpb.UninterpretedOption, nameIndex int, pathPrefix []int32) (path []int32, err error) {
	var fld *desc.FieldDescriptor
	nm := opt.GetName()[nameIndex]
	node := res.getOptionNamePartNode(nm)
	if nm.GetIsExtension() {
		extName := nm.GetNamePart()
		if extName[0] == '.' {
			extName = extName[1:] /* skip leading dot */
		}
		fld = l.findExtension(res.fd, extName, false, map[*dpb.FileDescriptorProto]struct{}{})
		if fld == nil {
			return nil, res.errs.handleErrorWithPos(node.start(),
				"%vunrecognized extension %s of %s",
				mc, extName, dm.GetMessageDescriptor().GetFullyQualifiedName())
		}
		if fld.GetOwner().GetFullyQualifiedName() != dm.GetMessageDescriptor().GetFullyQualifiedName() {
			return nil, res.errs.handleErrorWithPos(node.start(),
				"%vextension %s should extend %s but instead extends %s",
				mc, extName, dm.GetMessageDescriptor().GetFullyQualifiedName(), fld.GetOwner().GetFullyQualifiedName())
		}
	} else {
		fld = dm.GetMessageDescriptor().FindFieldByName(nm.GetNamePart())
		if fld == nil {
			return nil, res.errs.handleErrorWithPos(node.start(),
				"%vfield %s of %s does not exist",
				mc, nm.GetNamePart(), dm.GetMessageDescriptor().GetFullyQualifiedName())
		}
	}

	path = append(pathPrefix, fld.GetNumber())

	if len(opt.GetName()) > nameIndex+1 {
		nextnm := opt.GetName()[nameIndex+1]
		nextnode := res.getOptionNamePartNode(nextnm)
		if fld.GetType() != dpb.FieldDescriptorProto_TYPE_MESSAGE {
			return nil, res.errs.handleErrorWithPos(nextnode.start(),
				"%vcannot set field %s because %s is not a message",
				mc, nextnm.GetNamePart(), nm.GetNamePart())
		}
		if fld.IsRepeated() {
			return nil, res.errs.handleErrorWithPos(nextnode.start(),
				"%vcannot set field %s because %s is repeated (must use an aggregate)",
				mc, nextnm.GetNamePart(), nm.GetNamePart())
		}
		var fdm *dynamic.Message
		var err error
		if dm.HasField(fld) {
			var v interface{}
			v, err = dm.TryGetField(fld)
			fdm, _ = v.(*dynamic.Message)
		} else {
			fdm = dynamic.NewMessage(fld.GetMessageType())
			err = dm.TrySetField(fld, fdm)
		}
		if err != nil {
			return nil, res.errs.handleError(ErrorWithSourcePos{Pos: node.start(), Underlying: err})
		}
		// recurse to set next part of name
		return l.interpretField(res, mc, element, fdm, opt, nameIndex+1, path)
	}

	optNode := res.getOptionNode(opt)
	if err := l.setOptionField(res, mc, dm, fld, node, optNode.getValue()); err != nil {
		return nil, res.errs.handleError(err)
	}
	if fld.IsRepeated() {
		path = append(path, int32(dm.FieldLength(fld))-1)
	}
	return path, nil
}

func (l *linker) findExtension(fd *dpb.FileDescriptorProto, name string, public bool, checked map[*dpb.FileDescriptorProto]struct{}) *desc.FieldDescriptor {
	if _, ok := checked[fd]; ok {
		return nil
	}
	checked[fd] = struct{}{}
	d := fd.FindSymbol(name)
	if d != nil {
		if fld, ok := d.(*desc.FieldDescriptor); ok {
			return fld
		}
		return nil
	}

	// When public = false, we are searching only directly imported symbols. But we
	// also need to search transitive public imports due to semantics of public imports.
	if public {
		for _, dep := range fd.GetPublicDependencies() {
			d := l.findExtension(dep, name, true, checked)
			if d != nil {
				return d
			}
		}
	} else {
		for _, dep := range fd.GetDependencies() {
			d := l.findExtension(dep, name, true, checked)
			if d != nil {
				return d
			}
		}
	}
	return nil
}

func (l *linker) setOptionField(res *parseResult, mc *messageContext, dm *dynamic.Message, fld *dpb.FieldDescriptorProto, name node, val valueNode) error {
	v := val.value()
	if sl, ok := v.([]valueNode); ok {
		// handle slices a little differently than the others
		if fld.GetLabel() != dpb.FieldDescriptorProto_LABEL_REPEATED {
			return errorWithPos(val.start(), "%vvalue is an array but field is not repeated", mc)
		}
		origPath := mc.optAggPath
		defer func() {
			mc.optAggPath = origPath
		}()
		for index, item := range sl {
			mc.optAggPath = fmt.Sprintf("%s[%d]", origPath, index)
			if v, err := l.fieldValue(res, mc, fld, item, false); err != nil {
				return err
			} else if err = dm.TryAddRepeatedField(fld, v); err != nil {
				return errorWithPos(val.start(), "%verror setting value: %s", mc, err)
			}
		}
		return nil
	}

	v, err := l.fieldValue(res, mc, fld, val, false)
	if err != nil {
		return err
	}
	if fld.GetLabel() == dpb.FieldDescriptorProto_LABEL_REPEATED {
		err = dm.TryAddRepeatedField(fld, v)
	} else {
		if dm.HasField(fld) {
			return errorWithPos(name.start(), "%vnon-repeated option field %s already set", mc, fieldName(fld))
		}
		err = dm.TrySetField(fld, v)
	}
	if err != nil {
		return errorWithPos(val.start(), "%verror setting value: %s", mc, err)
	}

	return nil
}

func findOption(res *parseResult, scope string, opts []*dpb.UninterpretedOption, name string) (int, error) {
	found := -1
	for i, opt := range opts {
		if len(opt.Name) != 1 {
			continue
		}
		if opt.Name[0].GetIsExtension() || opt.Name[0].GetNamePart() != name {
			continue
		}
		if found >= 0 {
			optNode := res.getOptionNode(opt)
			return -1, res.errs.handleErrorWithPos(optNode.getName().start(), "%s: option %s cannot be defined more than once", scope, name)
		}
		found = i
	}
	return found, nil
}

func removeOption(uo []*dpb.UninterpretedOption, indexToRemove int) []*dpb.UninterpretedOption {
	if indexToRemove == 0 {
		return uo[1:]
	} else if indexToRemove == len(uo)-1 {
		return uo[:len(uo)-1]
	} else {
		return append(uo[:indexToRemove], uo[indexToRemove+1:]...)
	}
}

type messageContext struct {
	res         *parseResult
	file        *dpb.FileDescriptorProto
	elementType string
	elementName string
	option      *dpb.UninterpretedOption
	optAggPath  string
}

func (c *messageContext) String() string {
	var ctx bytes.Buffer
	if c.elementType != "file" {
		_, _ = fmt.Fprintf(&ctx, "%s %s: ", c.elementType, c.elementName)
	}
	if c.option != nil && c.option.Name != nil {
		ctx.WriteString("option ")
		writeOptionName(&ctx, c.option.Name)
		if c.res.nodes == nil {
			// if we have no source position info, try to provide as much context
			// as possible (if nodes != nil, we don't need this because any errors
			// will actually have file and line numbers)
			if c.optAggPath != "" {
				_, _ = fmt.Fprintf(&ctx, " at %s", c.optAggPath)
			}
		}
		ctx.WriteString(": ")
	}
	return ctx.String()
}

func writeOptionName(buf *bytes.Buffer, parts []*dpb.UninterpretedOption_NamePart) {
	first := true
	for _, p := range parts {
		if first {
			first = false
		} else {
			buf.WriteByte('.')
		}
		nm := p.GetNamePart()
		if nm[0] == '.' {
			// skip leading dot
			nm = nm[1:]
		}
		if p.GetIsExtension() {
			buf.WriteByte('(')
			buf.WriteString(nm)
			buf.WriteByte(')')
		} else {
			buf.WriteString(nm)
		}
	}
}

func fieldName(fld *desc.FieldDescriptor) string {
	if fld.IsExtension() {
		return fld.GetFullyQualifiedName()
	} else {
		return fld.GetName()
	}
}

func valueKind(val interface{}) string {
	switch val := val.(type) {
	case identifier:
		return "identifier"
	case bool:
		return "bool"
	case int64:
		if val < 0 {
			return "negative integer"
		}
		return "integer"
	case uint64:
		return "integer"
	case float64:
		return "double"
	case string, []byte:
		return "string"
	case []*aggregateEntryNode:
		return "message"
	default:
		return fmt.Sprintf("%T", val)
	}
}

func (l *linker) fieldValue(res *parseResult, mc *messageContext, fld *dpb.FieldDescriptorProto, val valueNode, enumAsString bool) (interface{}, error) {
	v := val.value()
	t := fld.GetType()
	switch t {
	case dpb.FieldDescriptorProto_TYPE_ENUM:
		if id, ok := v.(identifier); ok {
			ev := fld.GetEnumType().FindValueByName(string(id))
			if ev == nil {
				return nil, errorWithPos(val.start(), "%venum %s has no value named %s", mc, fld.GetEnumType().GetFullyQualifiedName(), id)
			}
			if enumAsString {
				return ev.GetName(), nil
			} else {
				return ev.GetNumber(), nil
			}
		}
		return nil, errorWithPos(val.start(), "%vexpecting enum, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_MESSAGE, dpb.FieldDescriptorProto_TYPE_GROUP:
		if aggs, ok := v.([]*aggregateEntryNode); ok {
			fmd := fld.GetMessageType()
			fdm := dynamic.NewMessage(fmd)
			origPath := mc.optAggPath
			defer func() {
				mc.optAggPath = origPath
			}()
			for _, a := range aggs {
				if origPath == "" {
					mc.optAggPath = a.name.value()
				} else {
					mc.optAggPath = origPath + "." + a.name.value()
				}
				var ffld *desc.FieldDescriptor
				if a.name.isExtension {
					n := a.name.name.val
					ffld = l.findExtension(mc.file, n, false, map[*dpb.FileDescriptorProto]struct{}{})
					if ffld == nil {
						// may need to qualify with package name
						pkg := mc.file.GetPackage()
						if pkg != "" {
							ffld = l.findExtension(mc.file, pkg+"."+n, false, map[*dpb.FileDescriptorProto]struct{}{})
						}
					}
				} else {
					ffld = fmd.FindFieldByName(a.name.value())
				}
				if ffld == nil {
					return nil, errorWithPos(val.start(), "%vfield %s not found", mc, a.name.name.val)
				}
				if err := l.setOptionField(res, mc, fdm, ffld, a.name, a.val); err != nil {
					return nil, err
				}
			}
			return fdm, nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting message, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_BOOL:
		if b, ok := v.(bool); ok {
			return b, nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting bool, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_BYTES:
		if str, ok := v.(string); ok {
			return []byte(str), nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting bytes, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_STRING:
		if str, ok := v.(string); ok {
			return str, nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting string, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_INT32, dpb.FieldDescriptorProto_TYPE_SINT32, dpb.FieldDescriptorProto_TYPE_SFIXED32:
		if i, ok := v.(int64); ok {
			if i > math.MaxInt32 || i < math.MinInt32 {
				return nil, errorWithPos(val.start(), "%vvalue %d is out of range for int32", mc, i)
			}
			return int32(i), nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxInt32 {
				return nil, errorWithPos(val.start(), "%vvalue %d is out of range for int32", mc, ui)
			}
			return int32(ui), nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting int32, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_UINT32, dpb.FieldDescriptorProto_TYPE_FIXED32:
		if i, ok := v.(int64); ok {
			if i > math.MaxUint32 || i < 0 {
				return nil, errorWithPos(val.start(), "%vvalue %d is out of range for uint32", mc, i)
			}
			return uint32(i), nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxUint32 {
				return nil, errorWithPos(val.start(), "%vvalue %d is out of range for uint32", mc, ui)
			}
			return uint32(ui), nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting uint32, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_INT64, dpb.FieldDescriptorProto_TYPE_SINT64, dpb.FieldDescriptorProto_TYPE_SFIXED64:
		if i, ok := v.(int64); ok {
			return i, nil
		}
		if ui, ok := v.(uint64); ok {
			if ui > math.MaxInt64 {
				return nil, errorWithPos(val.start(), "%vvalue %d is out of range for int64", mc, ui)
			}
			return int64(ui), nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting int64, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_UINT64, dpb.FieldDescriptorProto_TYPE_FIXED64:
		if i, ok := v.(int64); ok {
			if i < 0 {
				return nil, errorWithPos(val.start(), "%vvalue %d is out of range for uint64", mc, i)
			}
			return uint64(i), nil
		}
		if ui, ok := v.(uint64); ok {
			return ui, nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting uint64, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_DOUBLE:
		if d, ok := v.(float64); ok {
			return d, nil
		}
		if i, ok := v.(int64); ok {
			return float64(i), nil
		}
		if u, ok := v.(uint64); ok {
			return float64(u), nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting double, got %s", mc, valueKind(v))
	case dpb.FieldDescriptorProto_TYPE_FLOAT:
		if d, ok := v.(float64); ok {
			if (d > math.MaxFloat32 || d < -math.MaxFloat32) && !math.IsInf(d, 1) && !math.IsInf(d, -1) && !math.IsNaN(d) {
				return nil, errorWithPos(val.start(), "%vvalue %f is out of range for float", mc, d)
			}
			return float32(d), nil
		}
		if i, ok := v.(int64); ok {
			return float32(i), nil
		}
		if u, ok := v.(uint64); ok {
			return float32(u), nil
		}
		return nil, errorWithPos(val.start(), "%vexpecting float, got %s", mc, valueKind(v))
	default:
		return nil, errorWithPos(val.start(), "%vunrecognized field type: %s", mc, t)
	}
}
