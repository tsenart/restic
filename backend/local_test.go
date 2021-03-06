package backend_test

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"testing"

	"github.com/restic/restic/backend"
)

var testCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")

var TestStrings = []struct {
	id   string
	data string
}{
	{"c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2", "foobar"},
	{"248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1", "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"},
	{"cc5d46bdb4991c6eae3eb739c9c8a7a46fe9654fab79c47b4fe48383b5b25e1c", "foo/bar"},
	{"4e54d2c721cbdb730f01b10b62dec622962b36966ec685880effa63d71c808f2", "foo/../../baz"},
}

func setupLocalBackend(t *testing.T) *backend.Local {
	tempdir, err := ioutil.TempDir("", "restic-test-")
	ok(t, err)

	b, err := backend.CreateLocal(tempdir)
	ok(t, err)

	t.Logf("created local backend at %s", tempdir)

	return b
}

func teardownLocalBackend(t *testing.T, b *backend.Local) {
	if !*testCleanup {
		t.Logf("leaving local backend at %s\n", b.Location())
		return
	}

	ok(t, b.Delete())
}

func testBackend(b backend.Backend, t *testing.T) {
	for _, tpe := range []backend.Type{backend.Data, backend.Key, backend.Lock, backend.Snapshot, backend.Tree} {
		// detect non-existing files
		for _, test := range TestStrings {
			id, err := backend.ParseID(test.id)
			ok(t, err)

			// test if blob is already in repository
			ret, err := b.Test(tpe, id)
			ok(t, err)
			assert(t, !ret, "blob was found to exist before creating")

			// try to open not existing blob
			d, err := b.Get(tpe, id)
			assert(t, err != nil && d == nil, "blob data could be extracted befor creation")

			// try to get string out, should fail
			ret, err = b.Test(tpe, id)
			ok(t, err)
			assert(t, !ret, fmt.Sprintf("id %q was found (but should not have)", test.id))
		}

		// add files
		for _, test := range TestStrings {
			// store string in backend
			blob, err := b.Create(tpe)
			ok(t, err)

			_, err = blob.Write([]byte(test.data))
			ok(t, err)
			ok(t, blob.Close())

			id, err := blob.ID()
			ok(t, err)

			equals(t, test.id, id.String())

			// try to get it out again
			buf, err := b.Get(tpe, id)
			ok(t, err)
			assert(t, buf != nil, "Get() returned nil")

			// compare content
			equals(t, test.data, string(buf))

			// compare content again via stream function
			rd, err := b.GetReader(tpe, id)
			ok(t, err)
			buf, err = ioutil.ReadAll(rd)
			ok(t, err)
			equals(t, test.data, string(buf))
		}

		// test adding the first file again
		test := TestStrings[0]
		id, err := backend.ParseID(test.id)
		ok(t, err)

		// create blob
		blob, err := b.Create(tpe)
		ok(t, err)

		_, err = io.Copy(blob, bytes.NewReader([]byte(test.data)))
		ok(t, err)
		err = blob.Close()
		assert(t, err == backend.ErrAlreadyPresent,
			"wrong error returned: expected %v, got %v",
			backend.ErrAlreadyPresent, err)

		id2, err := blob.ID()
		ok(t, err)

		assert(t, id.Equal(id2), "IDs do not match: expected %v, got %v", id, id2)

		// remove and recreate
		err = b.Remove(tpe, id)
		ok(t, err)

		// create blob
		blob, err = b.Create(tpe)
		ok(t, err)

		_, err = io.Copy(blob, bytes.NewReader([]byte(test.data)))
		ok(t, err)
		err = blob.Close()
		ok(t, err)

		id2, err = blob.ID()
		ok(t, err)
		assert(t, id.Equal(id2), "IDs do not match: expected %v, got %v", id, id2)

		// list items
		IDs := backend.IDs{}

		for _, test := range TestStrings {
			id, err := backend.ParseID(test.id)
			ok(t, err)
			IDs = append(IDs, id)
		}

		ids, err := b.List(tpe)
		ok(t, err)

		sort.Sort(ids)
		sort.Sort(IDs)
		equals(t, IDs, ids)

		// remove content if requested
		if *testCleanup {
			for _, test := range TestStrings {
				id, err := backend.ParseID(test.id)
				ok(t, err)

				found, err := b.Test(tpe, id)
				ok(t, err)
				assert(t, found, fmt.Sprintf("id %q was not found before removal", id))

				ok(t, b.Remove(tpe, id))

				found, err = b.Test(tpe, id)
				ok(t, err)
				assert(t, !found, fmt.Sprintf("id %q not found after removal", id))
			}
		}

	}
}

func TestBackend(t *testing.T) {
	// test for non-existing backend
	b, err := backend.OpenLocal("/invalid-restic-test")
	assert(t, err != nil, "opening invalid repository at /invalid-restic-test should have failed, but err is nil")
	assert(t, b == nil, fmt.Sprintf("opening invalid repository at /invalid-restic-test should have failed, but b is not nil: %v", b))

	s := setupLocalBackend(t)
	defer teardownLocalBackend(t, s)

	testBackend(s, t)
}

func TestLocalBackendCreationFailures(t *testing.T) {
	b := setupLocalBackend(t)
	defer teardownLocalBackend(t, b)

	// test failure to create a new repository at the same location
	b2, err := backend.CreateLocal(b.Location())
	assert(t, err != nil && b2 == nil, fmt.Sprintf("creating a repository at %s for the second time should have failed", b.Location()))

	// test failure to create a new repository at the same location without a config file
	b2, err = backend.CreateLocal(b.Location())
	assert(t, err != nil && b2 == nil, fmt.Sprintf("creating a repository at %s for the second time should have failed", b.Location()))
}
