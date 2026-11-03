package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestECJEnumValuesArrayCopyDecompiles(t *testing.T) {
	classes := []string{
		"com/landray/kmss/common/module/util/WarnLevel.class",
		"com/landray/kmss/code/springmvc/trans/FileTranser$Result.class",
		"com/landray/kmss/constant/SysAuthConstant$AllCheck.class",
		"com/landray/kmss/constant/SysAuthConstant$AreaIsolation.class",
		"com/landray/kmss/constant/SysAuthConstant$AuthCheck.class",
		"com/landray/kmss/constant/SysAuthConstant$CheckType.class",
		"com/landray/kmss/constant/SysAuthConstant$AreaCheck.class",
		"com/landray/kmss/constant/SysAuthConstant$SoftDeleteCheck.class",
		"com/landray/kmss/gateway/contacts/rest/GatewayContactsSysEnum.class",
		"com/landray/kmss/km/calendar/ical/values/Frequency.class",
		"com/landray/kmss/km/calendar/ical/values/Weekday.class",
		"com/landray/kmss/km/calendar/ical/values/IcalValueType.class",
		"com/landray/kmss/km/pindagate/util/PindagateTypeEnum.class",
		"com/landray/kmss/kms/knowledge/util/KmsKnowledgeAnalysis.class",
		"com/landray/kmss/sys/ui/taglib/template/TemplateContext$Status.class",
		"com/landray/kmss/sys/ui/util/ForSystemType.class",
		"com/landray/kmss/util/HQLHelper$ORDERTYPE.class",
		"com/landray/kmss/util/RecurrenceUtil$RecurrenceMonthType.class",
		"com/landray/kmss/util/HQLHelper$SQLTYPE.class",
		"com/landray/kmss/util/excel/CellStyleEnum.class",
		"com/landray/kmss/util/RecurrenceUtil$RecurrenceEndType.class",
		"com/landray/kmss/util/RecurrenceUtil$RecurrenceFreq.class",
		"com/landray/kmss/util/breaker/CircuitBreakerState.class",
		"com/landray/kmss/util/ical/values/Frequency.class",
		"com/landray/kmss/util/ical/values/Weekday.class",
		"com/landray/kmss/util/ical/values/IcalValueType.class",
	}
	root := "/home/wlz/Target/ekp/hotdeploy/ekp/WEB-INF/classes/"
	for _, rel := range classes {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(root + rel)
			if err != nil {
				t.Skip(err)
			}
			src, err := Decompile(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(src, "enum ") {
				t.Fatalf("not an enum:\n%s", src)
			}
			if strings.Contains(src, "ENUM$VALUES") {
				t.Fatalf("synthetic backing array leaked:\n%s", src)
			}
		})
	}
}

func TestECJEnumConstantBodyStaysSource(t *testing.T) {
	const classPath = "/home/wlz/Target/ekp/hotdeploy/ekp/WEB-INF/classes/com/landray/kmss/spi/query/SearchType.class"
	raw, err := os.ReadFile(classPath)
	if err != nil {
		t.Skip(err)
	}
	result, err := DecompileWithOptions(raw, DecompileOptions{RetainUnprovedSource: true})
	if err != nil {
		t.Fatal(err)
	}
	src := result.Source
	if !strings.Contains(src, "class SearchType") && !strings.Contains(src, "enum SearchType") {
		t.Fatalf("missing type:\n%s", src)
	}
	if !strings.Contains(src, "NE") || !strings.Contains(src, "LIST") {
		t.Fatalf("missing constants:\n%s", src)
	}
}
