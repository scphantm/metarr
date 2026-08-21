package workflow

import "testing"

// TestEachFileTransformIsRegistered confirms the ETL transform (design.md
// §4.3) that lets a path.dir output reach a path.file input is present and
// correctly classified.
func TestEachFileTransformIsRegistered(t *testing.T) {
	transform, found := FindTransform("eachFile")
	if !found {
		t.Fatal("eachFile is not registered")
	}
	if transform.From != TypePathDir || transform.To != TypePathFile {
		t.Errorf("eachFile = {From: %q, To: %q}, want {From: %q, To: %q}",
			transform.From, transform.To, TypePathDir, TypePathFile)
	}
	if !transform.ImpliesIteration {
		t.Error("eachFile.ImpliesIteration = false, want true")
	}

	found = false
	for _, entry := range Transforms() {
		if entry.Name == "eachFile" {
			found = true
		}
	}
	if !found {
		t.Error("Transforms() does not include eachFile")
	}
}

// TestPathDirAndPathFileRemainSiblings confirms adding eachFile did not
// accidentally introduce a subtyping relationship between path.dir and
// path.file — they're connectable only via the explicit transform, never
// implicitly.
func TestPathDirAndPathFileRemainSiblings(t *testing.T) {
	if IsSubtypeOf(TypePathDir, TypePathFile) {
		t.Error("TypePathDir is a subtype of TypePathFile; want false")
	}
	if IsSubtypeOf(TypePathFile, TypePathDir) {
		t.Error("TypePathFile is a subtype of TypePathDir; want false")
	}
}

// TestCanConnectPathDirToPathFileOffersEachFile is the connect-time check
// the editor relies on: previously refused outright, now offered via the
// one new transform, not a direct/implicit match.
func TestCanConnectPathDirToPathFileOffersEachFile(t *testing.T) {
	connection := CanConnect(TypePathDir, TypePathFile)
	if connection.Direct {
		t.Fatal("CanConnect(path.dir, path.file).Direct = true, want false (must go through an explicit transform)")
	}

	found := false
	for _, candidate := range connection.Candidates {
		if candidate.Name == "eachFile" {
			found = true
		}
	}
	if !found {
		t.Errorf("Candidates = %+v, want eachFile among them", connection.Candidates)
	}

	auto, ok := connection.AutoApply()
	if !ok || auto.Name != "eachFile" {
		t.Errorf("AutoApply() = (%+v, %v), want (eachFile, true) — it's the only, unambiguous candidate", auto, ok)
	}
}

// TestPathListFamilyReusesGenericList confirms path.list/path.list.directory/
// path.list.file are exactly list<path>/list<path.dir>/list<path.file> —
// no new leaf types — and that covariance gives "path.list accepts either a
// directory list or a file list" for free.
func TestPathListFamilyReusesGenericList(t *testing.T) {
	if TypePathList != "list<path>" {
		t.Errorf("TypePathList = %q, want %q", TypePathList, "list<path>")
	}
	if TypePathListDir != "list<path.dir>" {
		t.Errorf("TypePathListDir = %q, want %q", TypePathListDir, "list<path.dir>")
	}
	if TypePathListFile != "list<path.file>" {
		t.Errorf("TypePathListFile = %q, want %q", TypePathListFile, "list<path.file>")
	}

	if !IsSubtypeOf(TypePathListDir, TypePathList) {
		t.Error("TypePathListDir is not a subtype of TypePathList; want true (covariance)")
	}
	if !IsSubtypeOf(TypePathListFile, TypePathList) {
		t.Error("TypePathListFile is not a subtype of TypePathList; want true (covariance)")
	}
	// The reverse must not hold: a bare path.list cannot narrow to
	// directory-only or file-only without a transform.
	if IsSubtypeOf(TypePathList, TypePathListDir) {
		t.Error("TypePathList is a subtype of TypePathListDir; want false")
	}
}

// TestCanConnectSupertypeToSubtypeIsUnsafeButAllowed confirms the narrowing
// direction (a supertype output reaching a subtype input, e.g. a bare path
// reaching a path.dir socket) is now allowed directly, but flagged. Checked
// against two different pairs in the family to show the rule is structural
// (IsSubtypeOf(to, from)), not hardcoded to the path root.
func TestCanConnectSupertypeToSubtypeIsUnsafeButAllowed(t *testing.T) {
	connection := CanConnect(TypePath, TypePathDir)
	if !connection.Direct {
		t.Error("CanConnect(path, path.dir).Direct = false, want true")
	}
	if !connection.TypeUnsafe {
		t.Error("CanConnect(path, path.dir).TypeUnsafe = false, want true")
	}

	connection = CanConnect(TypePathFile, TypePathFileVideo)
	if !connection.Direct || !connection.TypeUnsafe {
		t.Errorf("CanConnect(path.file, path.file.video) = %+v, want {Direct: true, TypeUnsafe: true}", connection)
	}
}

// TestCanConnectSubtypeToSupertypeStaysSafe is a regression guard: the
// pre-existing, safe covariant direction must never start setting
// TypeUnsafe.
func TestCanConnectSubtypeToSupertypeStaysSafe(t *testing.T) {
	connection := CanConnect(TypePathDir, TypePath)
	if !connection.Direct {
		t.Error("CanConnect(path.dir, path).Direct = false, want true")
	}
	if connection.TypeUnsafe {
		t.Error("CanConnect(path.dir, path).TypeUnsafe = true, want false")
	}
}

// TestCanConnectUnrelatedTypesStayUnaffected confirms the new branch only
// fires on an actual subtype relationship — an unrelated pair bridged by an
// explicit transform must not be marked TypeUnsafe.
func TestCanConnectUnrelatedTypesStayUnaffected(t *testing.T) {
	connection := CanConnect(TypeString, TypeNumber)
	if connection.Direct {
		t.Error("CanConnect(string, number).Direct = true, want false")
	}
	if connection.TypeUnsafe {
		t.Error("CanConnect(string, number).TypeUnsafe = true, want false")
	}

	found := false
	for _, candidate := range connection.Candidates {
		if candidate.Name == "parseNumber" {
			found = true
		}
	}
	if !found {
		t.Errorf("Candidates = %+v, want parseNumber among them", connection.Candidates)
	}
}
