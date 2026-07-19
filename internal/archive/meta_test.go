package archive

import "testing"

func TestParseMetaAcceptsUnityYAML(t *testing.T) {
	data := []byte(`fileFormatVersion: 2
guid: ABCDEF0123456789ABCDEF0123456789
folderAsset: yes
SomeImporter:
  platformData:
  - first:
      : Any
`)
	meta, err := ParseMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.GUID != "abcdef0123456789abcdef0123456789" || !meta.IsFolder {
		t.Fatalf("unexpected meta: %#v", meta)
	}
}

func TestParseMetaRequiresExactlyOneGUID(t *testing.T) {
	if _, err := ParseMeta([]byte("folderAsset: yes\n")); err == nil {
		t.Fatal("expected missing GUID error")
	}
	data := []byte("guid: 11111111111111111111111111111111\nguid: 22222222222222222222222222222222\n")
	if _, err := ParseMeta(data); err == nil {
		t.Fatal("expected duplicate GUID fields to be rejected")
	}
}
