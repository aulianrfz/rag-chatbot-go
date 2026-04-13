// internal/rag/types.go
// Struct untuk parsing data JSON produk dan transaksi.

package rag

// Product merepresentasikan satu produk dari products.json.
type Product struct {
	ID        string   `json:"id"`
	Nama      string   `json:"nama"`
	Kategori  string   `json:"kategori"`
	Harga     int64    `json:"harga"`
	Stok      int      `json:"stok"`
	Deskripsi string   `json:"deskripsi"`
	Tags      []string `json:"tags"`
}

// Purchase merepresentasikan satu transaksi dari purchases.json.
type Purchase struct {
	TransaksiID string `json:"transaksi_id"`
	Tanggal     string `json:"tanggal"`
	ProdukID    string `json:"produk_id"`
	ProdukNama  string `json:"produk_nama"`
	Kategori    string `json:"kategori"`
	Jumlah      int    `json:"jumlah"`
	Total       int64  `json:"total"`
}

// Document adalah unit terkecil yang diindeks untuk pencarian.
type Document struct {
	ID        string
	Content   string
	Score     float64
	Embedding []float32 // vektor hasil embedding (diisi saat indexing)
}