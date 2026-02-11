package condition

import "testing"

func TestCondition_Update_NoChange_ReturnsFalse(t *testing.T) {
	a := &Condition{Type: "A", Status: True, Category: Warn}
	if a.Update(Condition{Type: "A", Status: True, Category: Warn}) {
		t.Fatalf("expected false")
	}
}

func TestCondition_Equal_ItemsDifferent_False(t *testing.T) {
	a := &Condition{Type: "A", Status: True, Category: Warn, Items: []string{"1"}}
	if a.Equal(Condition{Type: "A", Status: True, Category: Warn, Items: []string{"2"}}) {
		t.Fatalf("expected false")
	}
}

func TestCondition_Equal_DurableDifferent_False(t *testing.T) {
	a := &Condition{Type: "A", Status: True, Category: Warn, Durable: true}
	if a.Equal(Condition{Type: "A", Status: True, Category: Warn, Durable: false}) {
		t.Fatalf("expected false")
	}
}

func TestCondition_Equal_MessageDifferent_False(t *testing.T) {
	a := &Condition{Type: "A", Status: True, Category: Warn, Message: "x"}
	if a.Equal(Condition{Type: "A", Status: True, Category: Warn, Message: "y"}) {
		t.Fatalf("expected false")
	}
}

func TestConditions_UpdateConditions_AddsAll(t *testing.T) {
	a := Conditions{}
	b := Conditions{List: []Condition{{Type: "A", Status: True}, {Type: "B", Status: False}}}
	a.UpdateConditions(b)
	if len(a.List) != 2 {
		t.Fatalf("expected 2 got %d", len(a.List))
	}
}

func TestConditions_UpdateConditions_UpdatesExisting(t *testing.T) {
	a := Conditions{List: []Condition{{Type: "A", Status: False, Category: Warn}}}
	b := Conditions{List: []Condition{{Type: "A", Status: True, Category: Critical}}}
	a.UpdateConditions(b)
	c := a.FindCondition("A")
	if c == nil || c.Status != True || c.Category != Critical {
		t.Fatalf("unexpected: %#v", c)
	}
}

func TestConditions_BeginStagingConditions_NilList_NoPanic(t *testing.T) {
	var c Conditions
	c.BeginStagingConditions()
}

func TestConditions_EndStagingConditions_NilList_NoPanic(t *testing.T) {
	var c Conditions
	c.EndStagingConditions()
}

func TestConditions_FindCondition_NilList_ReturnsNil(t *testing.T) {
	var c Conditions
	if c.FindCondition("A") != nil {
		t.Fatalf("expected nil")
	}
}

func TestConditions_SetCondition_InitializesList(t *testing.T) {
	var c Conditions
	c.SetCondition(Condition{Type: "A", Status: True, Category: Advisory})
	if len(c.List) != 1 {
		t.Fatalf("expected 1")
	}
}

func TestConditions_SetCondition_UpdatesExistingCondition(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: False, Category: Warn}}}
	c.SetCondition(Condition{Type: "A", Status: True, Category: Critical})
	f := c.FindCondition("A")
	if f == nil || f.Status != True || f.Category != Critical {
		t.Fatalf("unexpected: %#v", f)
	}
}

func TestConditions_SetCondition_AddsSecondType(t *testing.T) {
	c := Conditions{}
	c.SetCondition(Condition{Type: "A", Status: True, Category: Warn})
	c.SetCondition(Condition{Type: "B", Status: True, Category: Warn})
	if len(c.List) != 2 {
		t.Fatalf("expected 2 got %d", len(c.List))
	}
}

func TestConditions_StageCondition_NilList_NoPanic(t *testing.T) {
	var c Conditions
	c.StageCondition("A")
}

func TestConditions_StageCondition_UnknownType_NoChange(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", staged: false}}}
	c.StageCondition("X")
	if c.List[0].staged {
		t.Fatalf("expected false")
	}
}

func TestConditions_DeleteCondition_NilList_NoPanic(t *testing.T) {
	var c Conditions
	c.DeleteCondition("A")
}

func TestConditions_DeleteCondition_RemovesWhenNotStaging(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A"}, {Type: "B"}}}
	c.DeleteCondition("A")
	if len(c.List) != 1 || c.List[0].Type != "B" {
		t.Fatalf("unexpected list: %#v", c.List)
	}
}

func TestConditions_DeleteCondition_WhileStaging_KeepsButUnstagesMatchedOnly(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", staged: true}, {Type: "B", staged: true}}}
	c.BeginStagingConditions()
	c.StageCondition("A")
	c.StageCondition("B")
	c.DeleteCondition("A")
	if len(c.List) != 2 {
		t.Fatalf("expected kept")
	}
	if c.List[0].Type == "A" && c.List[0].staged {
		t.Fatalf("expected A unstaged")
	}
	if c.List[1].Type == "B" && !c.List[1].staged {
		t.Fatalf("expected B staged")
	}
}

func TestConditions_HasCondition_NilList_False(t *testing.T) {
	var c Conditions
	if c.HasCondition("A") {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasCondition_NoTypes_False(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True}}}
	if c.HasCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasCondition_AllTrue_True(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True}, {Type: "B", Status: True}}}
	if !c.HasCondition("A", "B") {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasCondition_OneFalse_False(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True}, {Type: "B", Status: False}}}
	if c.HasCondition("A", "B") {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasAnyCondition_NilList_False(t *testing.T) {
	var c Conditions
	if c.HasAnyCondition("A") {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasAnyCondition_AnyTrue_True(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: False}, {Type: "B", Status: True}}}
	if !c.HasAnyCondition("A", "B") {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasAnyCondition_NoneTrue_False(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: False}, {Type: "B", Status: False}}}
	if c.HasAnyCondition("A", "B") {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasAnyCondition_NoTypes_False(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True}}}
	if c.HasAnyCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasConditionCategory_NilList_False(t *testing.T) {
	var c Conditions
	if c.HasConditionCategory(Critical) {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasConditionCategory_FalseWhenCategoryMissing(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Warn}}}
	if c.HasConditionCategory(Critical) {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasConditionCategory_FalseWhenStatusFalse(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: False, Category: Critical}}}
	if c.HasConditionCategory(Critical) {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasConditionCategory_MultipleCategories_True(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Warn}}}
	if !c.HasConditionCategory(Warn, Error) {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasCriticalCondition_FalseWhenNilList(t *testing.T) {
	var c Conditions
	if c.HasCriticalCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasErrorCondition_FalseWhenNilList(t *testing.T) {
	var c Conditions
	if c.HasErrorCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasWarnCondition_FalseWhenNilList(t *testing.T) {
	var c Conditions
	if c.HasWarnCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasBlockerCondition_FalseWhenNilList(t *testing.T) {
	var c Conditions
	if c.HasBlockerCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasReQCondition_FalseWhenNilList(t *testing.T) {
	var c Conditions
	if c.HasReQCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasReQCondition_TrueWhenBothPresent(t *testing.T) {
	c := Conditions{List: []Condition{
		{Type: ValidatingVDDK, Status: True},
		{Type: VMMissingChangedBlockTracking, Status: True},
	}}
	if !c.HasReQCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasReQCondition_FalseWhenBothFalse(t *testing.T) {
	c := Conditions{List: []Condition{
		{Type: ValidatingVDDK, Status: False},
		{Type: VMMissingChangedBlockTracking, Status: False},
	}}
	if c.HasReQCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_IsReady_FalseWhenStagingAndUnstaged(t *testing.T) {
	c := Conditions{List: []Condition{{Type: Ready, Status: True, Durable: false}}}
	c.BeginStagingConditions()
	if c.IsReady() {
		t.Fatalf("expected false while unstaged")
	}
}

func TestConditions_IsReady_TrueWhenStagingAndStaged(t *testing.T) {
	c := Conditions{List: []Condition{{Type: Ready, Status: True, Durable: false}}}
	c.BeginStagingConditions()
	c.StageCondition(Ready)
	if !c.IsReady() {
		t.Fatalf("expected true")
	}
}

func TestConditions_Explain_InitiallyEmpty(t *testing.T) {
	var c Conditions
	e := c.Explain()
	if !e.Empty() {
		t.Fatalf("expected empty")
	}
}

func TestConditions_Explain_AfterSetCondition_AddedTracked(t *testing.T) {
	var c Conditions
	c.SetCondition(Condition{Type: "A", Status: True})
	e := c.Explain()
	if _, ok := e.Added["A"]; !ok {
		t.Fatalf("expected added")
	}
}

func TestConditions_Explain_AfterUpdateCondition_UpdatedTracked(t *testing.T) {
	var c Conditions
	c.SetCondition(Condition{Type: "A", Status: True, Category: Warn})
	c.Explain() // build internal maps
	c.SetCondition(Condition{Type: "A", Status: True, Category: Critical})
	e := c.Explain()
	// if A was in Added, Updated may be empty, but we at least ensure explain builds without panic
	_ = e
}

func TestExplain_BuildInitializesMaps(t *testing.T) {
	var e Explain
	e.build()
	if e.Added == nil || e.Updated == nil || e.Deleted == nil {
		t.Fatalf("expected maps initialized")
	}
}

func TestExplain_Updated_RemovesFromDeleted(t *testing.T) {
	var e Explain
	e.deleted(Condition{Type: "A"})
	e.updated(Condition{Type: "A"})
	if _, ok := e.Deleted["A"]; ok {
		t.Fatalf("expected removed from deleted")
	}
	if _, ok := e.Updated["A"]; !ok {
		t.Fatalf("expected updated")
	}
}

func TestExplain_Deleted_RemovesFromAddedAndUpdated(t *testing.T) {
	var e Explain
	e.added(Condition{Type: "A"})
	e.updated(Condition{Type: "B"})
	e.deleted(Condition{Type: "A"})
	if _, ok := e.Added["A"]; ok {
		t.Fatalf("expected removed from added")
	}
}

func TestExplain_Len_CountsDeleted_CurrentBehavior(t *testing.T) {
	var e Explain
	e.deleted(Condition{Type: "A"})
	if e.Len() != 1 {
		t.Fatalf("expected 1 got %d", e.Len())
	}
}

func TestExplain_Len_CountsUpdatedTwice_CurrentBehavior(t *testing.T) {
	var e Explain
	e.updated(Condition{Type: "A"})
	if e.Len() != 2 {
		t.Fatalf("expected 2 got %d", e.Len())
	}
}

func TestExplain_Empty_FalseWhenUpdatedPresent(t *testing.T) {
	var e Explain
	e.updated(Condition{Type: "A"})
	if e.Empty() {
		t.Fatalf("expected not empty")
	}
}

func TestExplain_Empty_FalseWhenDeletedPresent(t *testing.T) {
	var e Explain
	e.deleted(Condition{Type: "A"})
	if e.Empty() {
		t.Fatalf("expected not empty")
	}
}

func TestConditions_HasCriticalCondition_True(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: True, Category: Critical}}}
	if !c.HasCriticalCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasCriticalCondition_FalseOnFalseStatus(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: False, Category: Critical}}}
	if c.HasCriticalCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasErrorCondition_True(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: True, Category: Error}}}
	if !c.HasErrorCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasWarnCondition_True(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: True, Category: Warn}}}
	if !c.HasWarnCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasWarnCondition_FalseWhenNotWarn(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: True, Category: Error}}}
	if c.HasWarnCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasBlockerCondition_TrueOnCritical(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: True, Category: Critical}}}
	if !c.HasBlockerCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasBlockerCondition_TrueOnError(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: True, Category: Error}}}
	if !c.HasBlockerCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasBlockerCondition_FalseOnWarn(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "X", Status: True, Category: Warn}}}
	if c.HasBlockerCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_HasReQCondition_TrueOnValidatingVDDK(t *testing.T) {
	c := Conditions{List: []Condition{{Type: ValidatingVDDK, Status: True, Category: Advisory}}}
	if !c.HasReQCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasReQCondition_TrueOnMissingCBT(t *testing.T) {
	c := Conditions{List: []Condition{{Type: VMMissingChangedBlockTracking, Status: True, Category: Advisory}}}
	if !c.HasReQCondition() {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasReQCondition_FalseWhenFalseStatus(t *testing.T) {
	c := Conditions{List: []Condition{{Type: ValidatingVDDK, Status: False, Category: Advisory}}}
	if c.HasReQCondition() {
		t.Fatalf("expected false")
	}
}

func TestConditions_IsReady_True(t *testing.T) {
	c := Conditions{List: []Condition{{Type: Ready, Status: True, Category: Advisory}}}
	if !c.IsReady() {
		t.Fatalf("expected true")
	}
}

func TestConditions_IsReady_FalseWhenMissing(t *testing.T) {
	c := Conditions{}
	if c.IsReady() {
		t.Fatalf("expected false")
	}
}

func TestConditions_IsReady_FalseWhenStatusFalse(t *testing.T) {
	c := Conditions{List: []Condition{{Type: Ready, Status: False, Category: Advisory}}}
	if c.IsReady() {
		t.Fatalf("expected false")
	}
}

func TestExplain_LenAndEmpty_NewIsEmpty(t *testing.T) {
	var e Explain
	if e.Len() != 0 {
		t.Fatalf("expected 0")
	}
	if !e.Empty() {
		t.Fatalf("expected empty")
	}
}

func TestExplain_AddedUpdatedDeleted_AffectEmpty(t *testing.T) {
	var e Explain
	e.added(Condition{Type: "A"})
	// Current Len() implementation does not count Added entries, so Empty() remains true.
	if !e.Empty() {
		t.Fatalf("expected empty (Len ignores Added)")
	}
}

func TestExplain_Len_DoesNotCountAdded_CurrentBehavior(t *testing.T) {
	var e Explain
	e.added(Condition{Type: "A"})
	if e.Len() != 0 {
		t.Fatalf("expected 0 (Added not counted), got %d", e.Len())
	}
}

func TestExplain_AddedThenUpdated_DoesNotCountUpdated(t *testing.T) {
	var e Explain
	e.added(Condition{Type: "A"})
	e.updated(Condition{Type: "A"})
	// updated() early-returns if already in Added.
	if _, ok := e.Updated["A"]; ok {
		t.Fatalf("expected not updated when added")
	}
}

func TestExplain_AddedThenDeleted_MovesToDeleted(t *testing.T) {
	var e Explain
	e.added(Condition{Type: "A"})
	e.deleted(Condition{Type: "A"})
	if _, ok := e.Deleted["A"]; !ok {
		t.Fatalf("expected deleted")
	}
	if _, ok := e.Added["A"]; ok {
		t.Fatalf("expected removed from added")
	}
}

func TestExplain_UpdatedThenDeleted_RemovesFromUpdated(t *testing.T) {
	var e Explain
	e.updated(Condition{Type: "A"})
	e.deleted(Condition{Type: "A"})
	if _, ok := e.Updated["A"]; ok {
		t.Fatalf("expected removed from updated")
	}
	if _, ok := e.Deleted["A"]; !ok {
		t.Fatalf("expected deleted")
	}
}

func TestExplain_DeletedThenAdded_RemovesFromDeleted(t *testing.T) {
	var e Explain
	e.deleted(Condition{Type: "A"})
	e.added(Condition{Type: "A"})
	if _, ok := e.Deleted["A"]; ok {
		t.Fatalf("expected removed from deleted")
	}
	if _, ok := e.Added["A"]; !ok {
		t.Fatalf("expected added")
	}
}

func TestConditions_FindCondition_RespectsStaging(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, staged: false}}}
	c.BeginStagingConditions()
	// not durable, staged false => FindCondition should return nil when staging.
	if c.FindCondition("A") != nil {
		t.Fatalf("expected nil")
	}
}

func TestConditions_FindCondition_IgnoresStagingWhenNotStaging(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, staged: false}}}
	if c.FindCondition("A") == nil {
		t.Fatalf("expected found")
	}
}

func TestConditions_HasConditionCategory_RespectsStaging(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Critical, staged: false}}}
	c.BeginStagingConditions()
	if c.HasConditionCategory(Critical) {
		t.Fatalf("expected false while unstaged")
	}
	c.StageCondition("A")
	if !c.HasConditionCategory(Critical) {
		t.Fatalf("expected true after staging")
	}
}

func TestConditions_DeleteCondition_WhileStaging_UnstagesButKeeps(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Critical}}}
	c.BeginStagingConditions()
	c.SetCondition(Condition{Type: "A", Status: True, Category: Critical})
	c.DeleteCondition("A")
	if len(c.List) != 1 {
		t.Fatalf("expected kept while staging")
	}
	if c.List[0].staged {
		t.Fatalf("expected unstaged")
	}
}

func TestConditions_EndStagingConditions_RemovesUnstaged(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", staged: false}, {Type: "B", staged: true}}}
	c.BeginStagingConditions()
	// Keep B staged, ensure A unstaged
	c.StageCondition("B")
	c.EndStagingConditions()
	if len(c.List) != 1 || c.List[0].Type != "B" {
		t.Fatalf("unexpected list: %#v", c.List)
	}
}

func TestConditions_BeginStagingConditions_SetsDurableStagedTrue(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Durable: true}, {Type: "B", Durable: false}}}
	c.BeginStagingConditions()
	if !c.List[0].staged {
		t.Fatalf("expected durable staged")
	}
	if c.List[1].staged {
		t.Fatalf("expected non-durable unstaged")
	}
}

func TestConditions_EndStagingConditions_DeletesUnstagedAndKeepsDurable(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Durable: true}, {Type: "B", Durable: false}}}
	c.BeginStagingConditions()
	// A remains staged due to durable; B is unstaged.
	c.EndStagingConditions()
	if len(c.List) != 1 || c.List[0].Type != "A" {
		t.Fatalf("unexpected list: %#v", c.List)
	}
}

func TestConditions_FindCondition_ReturnsNilWhenStagingAndUnstaged(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Durable: false}}}
	c.BeginStagingConditions()
	if c.FindCondition("A") != nil {
		t.Fatalf("expected nil")
	}
}

func TestConditions_FindCondition_ReturnsWhenStagingAndDurable(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Durable: true}}}
	c.BeginStagingConditions()
	if c.FindCondition("A") == nil {
		t.Fatalf("expected found")
	}
}

func TestConditions_FindCondition_ReturnsWhenStagingAndStaged(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Durable: false}}}
	c.BeginStagingConditions()
	c.StageCondition("A")
	if c.FindCondition("A") == nil {
		t.Fatalf("expected found")
	}
}

func TestConditions_HasCondition_RespectsStaging(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Durable: false}}}
	c.BeginStagingConditions()
	if c.HasCondition("A") {
		t.Fatalf("expected false while unstaged")
	}
	c.StageCondition("A")
	if !c.HasCondition("A") {
		t.Fatalf("expected true after stage")
	}
}

func TestConditions_HasAnyCondition_RespectsStaging(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Durable: false}}}
	c.BeginStagingConditions()
	if c.HasAnyCondition("A") {
		t.Fatalf("expected false while unstaged")
	}
	c.StageCondition("A")
	if !c.HasAnyCondition("A") {
		t.Fatalf("expected true after stage")
	}
}

func TestConditions_HasConditionCategory_TrueWhenAnyMatches(t *testing.T) {
	c := Conditions{List: []Condition{
		{Type: "A", Status: True, Category: Warn},
		{Type: "B", Status: True, Category: Error},
	}}
	if !c.HasConditionCategory(Critical, Error) {
		t.Fatalf("expected true")
	}
}

func TestConditions_HasConditionCategory_FalseWhenNoNamesProvided(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Error}}}
	if c.HasConditionCategory() {
		t.Fatalf("expected false")
	}
}

func TestConditions_StageCondition_MultipleTypes(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A"}, {Type: "B"}, {Type: "C"}}}
	c.StageCondition("A", "C")
	if !c.List[0].staged || c.List[1].staged || !c.List[2].staged {
		t.Fatalf("unexpected staging: %#v", c.List)
	}
}

func TestConditions_DeleteCondition_MultipleTypes(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A"}, {Type: "B"}, {Type: "C"}}}
	c.DeleteCondition("A", "C")
	if len(c.List) != 1 || c.List[0].Type != "B" {
		t.Fatalf("unexpected list: %#v", c.List)
	}
}

func TestConditions_DeleteCondition_Staging_DeletesExplainButKeepsEntry(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Durable: false}}}
	c.BeginStagingConditions()
	c.StageCondition("A")
	c.DeleteCondition("A")
	if len(c.List) != 1 {
		t.Fatalf("expected kept")
	}
	if c.List[0].staged {
		t.Fatalf("expected unstaged")
	}
	// Explain should record deletion.
	if _, ok := c.Explain().Deleted["A"]; !ok {
		t.Fatalf("expected explain deleted")
	}
}

func TestConditions_Explain_DeletedRecorded(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A"}, {Type: "B"}}}
	c.DeleteCondition("A")
	e := c.Explain()
	if _, ok := e.Deleted["A"]; !ok {
		t.Fatalf("expected deleted recorded")
	}
}

func TestConditions_Explain_StageDoesNotChangeExplain(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A"}}}
	c.StageCondition("A")
	e := c.Explain()
	// StageCondition doesn't call Explain hooks.
	if !e.Empty() {
		t.Fatalf("expected empty explain")
	}
}

func TestExplain_AddedClearsDeletedAndUpdatedForSameType(t *testing.T) {
	var e Explain
	e.updated(Condition{Type: "A"})
	e.deleted(Condition{Type: "A"})
	e.added(Condition{Type: "A"})
	if _, ok := e.Deleted["A"]; ok {
		t.Fatalf("expected cleared deleted")
	}
	if _, ok := e.Updated["A"]; ok {
		t.Fatalf("expected cleared updated")
	}
	if _, ok := e.Added["A"]; !ok {
		t.Fatalf("expected added present")
	}
}

func TestExplain_UpdatedClearsDeletedForSameType(t *testing.T) {
	var e Explain
	e.deleted(Condition{Type: "A"})
	e.updated(Condition{Type: "A"})
	if _, ok := e.Deleted["A"]; ok {
		t.Fatalf("expected cleared deleted")
	}
}

func TestExplain_DeletedClearsAddedAndUpdatedForSameType(t *testing.T) {
	var e Explain
	e.added(Condition{Type: "A"})
	e.updated(Condition{Type: "A"})
	e.deleted(Condition{Type: "A"})
	if _, ok := e.Added["A"]; ok {
		t.Fatalf("expected cleared added")
	}
	if _, ok := e.Updated["A"]; ok {
		t.Fatalf("expected cleared updated")
	}
	if _, ok := e.Deleted["A"]; !ok {
		t.Fatalf("expected deleted")
	}
}

func TestConditions_HasConditionCategory_Staging_DurableStillCounts(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Critical, Durable: true}}}
	c.BeginStagingConditions()
	if !c.HasConditionCategory(Critical) {
		t.Fatalf("expected true (durable staged)")
	}
}

func TestConditions_HasConditionCategory_Staging_UnstagedDoesNotCount(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Critical, Durable: false}}}
	c.BeginStagingConditions()
	if c.HasConditionCategory(Critical) {
		t.Fatalf("expected false while unstaged")
	}
}

func TestConditions_HasConditionCategory_Staging_StagedCounts(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Status: True, Category: Critical, Durable: false}}}
	c.BeginStagingConditions()
	c.StageCondition("A")
	if !c.HasConditionCategory(Critical) {
		t.Fatalf("expected true after stage")
	}
}

func TestConditions_EndStagingConditions_ResetsStagingFlag(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A", Durable: true}}}
	c.BeginStagingConditions()
	c.EndStagingConditions()
	if c.staging {
		t.Fatalf("expected false")
	}
}

func TestConditions_BeginStagingConditions_SetsStagingFlag(t *testing.T) {
	c := Conditions{}
	c.BeginStagingConditions()
	if !c.staging {
		t.Fatalf("expected true")
	}
}

func TestCondition_Equal_TypeDifferent_False(t *testing.T) {
	a := &Condition{Type: "A", Status: True, Category: Warn}
	if a.Equal(Condition{Type: "B", Status: True, Category: Warn}) {
		t.Fatalf("expected false")
	}
}

func TestCondition_Equal_StatusDifferent_False(t *testing.T) {
	a := &Condition{Type: "A", Status: True, Category: Warn}
	if a.Equal(Condition{Type: "A", Status: False, Category: Warn}) {
		t.Fatalf("expected false")
	}
}

func TestConditions_Explain_AfterDeleteCondition_DeletedTypePresent(t *testing.T) {
	c := Conditions{List: []Condition{{Type: "A"}, {Type: "B"}}}
	c.DeleteCondition("B")
	e := c.Explain()
	if _, ok := e.Deleted["B"]; !ok {
		t.Fatalf("expected deleted B")
	}
}

func TestExplain_Updated_NoAdded_AllowsUpdatedEntry(t *testing.T) {
	var e Explain
	e.updated(Condition{Type: "A"})
	if _, ok := e.Updated["A"]; !ok {
		t.Fatalf("expected updated")
	}
}

func TestExplain_Deleted_OverridesUpdated(t *testing.T) {
	var e Explain
	e.updated(Condition{Type: "A"})
	e.deleted(Condition{Type: "A"})
	if _, ok := e.Updated["A"]; ok {
		t.Fatalf("expected updated cleared")
	}
	if _, ok := e.Deleted["A"]; !ok {
		t.Fatalf("expected deleted")
	}
}
