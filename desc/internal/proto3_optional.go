package internal

import (
	"strings"

	"github.com/golang/protobuf/proto"
	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// ProcessProto3OptionalFields adds synthetic oneofs to the given message descriptor
// for each proto3 optional field. It also updates the fields to have the correct
// oneof index reference.
func ProcessProto3OptionalFields(msgd *dpb.DescriptorProto) {
	var allNames map[string]struct{}
	for _, fd := range msgd.Field {
		if fd.GetProto3Optional() {
			// lazy init the set of all names
			if allNames == nil {
				allNames = map[string]struct{}{}
				for _, fd := range msgd.Field {
					allNames[fd.GetName()] = struct{}{}
				}
				for _, fd := range msgd.Extension {
					allNames[fd.GetName()] = struct{}{}
				}
				for _, ed := range msgd.EnumType {
					allNames[ed.GetName()] = struct{}{}
					for _, evd := range ed.Value {
						allNames[evd.GetName()] = struct{}{}
					}
				}
				for _, fd := range msgd.NestedType {
					allNames[fd.GetName()] = struct{}{}
				}
				for _, n := range msgd.ReservedName {
					allNames[n] = struct{}{}
				}
			}

			// Compute a name for the synthetic oneof. This uses the same
			// algorithm as used in protoc:
			//  https://github.com/protocolbuffers/protobuf/blob/74ad62759e0a9b5a21094f3fb9bb4ebfaa0d1ab8/src/google/protobuf/compiler/parser.cc#L785-L803
			ooName := fd.GetName()
			if !strings.HasPrefix(ooName, "_") {
				ooName = "_" + ooName
			}
			for {
				_, ok := allNames[ooName]
				if !ok {
					// found a unique name
					allNames[ooName] = struct{}{}
					break
				}
				ooName = "X" + ooName
			}

			fd.OneofIndex = proto.Int32(int32(len(msgd.OneofDecl)))
			msgd.OneofDecl = append(msgd.OneofDecl, &dpb.OneofDescriptorProto{Name: proto.String(ooName)})
		}
	}
}
