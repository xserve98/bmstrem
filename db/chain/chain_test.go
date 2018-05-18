package chain

import (
	"reflect"
	"sync"
	"testing"
)

func TestExpresionChain_Render(t *testing.T) {
	type fields struct {
		lock          sync.Mutex
		segments      []querySegmentAtom
		table         string
		mainOperation querySegmentAtom
		limit         *querySegmentAtom
		offset        *querySegmentAtom
	}

	tests := []struct {
		name     string
		chain    *ExpresionChain
		want     string
		wantArgs []interface{}
		wantErr  bool
	}{
		{
			name: "basic selection with where",
			chain: (&ExpresionChain{}).Select("field1", "field2", "field3").
				Table("convenient_table").
				Where("field1 > ?", 1).
				Where("field2 == ?", 2).
				Where("field3 > ?", "pajarito"),
			want:     "SELECT field1, field2, field3 FROM convenient_table WHERE field1 > $1 AND field2 == $2 AND field3 > $3",
			wantArgs: []interface{}{1, 2, "pajarito"},
			wantErr:  false,
		},
		{
			name: "basic selection with where and join",
			chain: (&ExpresionChain{}).Select("field1", "field2", "field3").
				Table("convenient_table").
				Where("field1 > ?", 1).
				Where("field2 == ?", 2).
				Where("field3 > ?", "pajarito").
				Join("another_convenient_table ON pirulo = ?", "unpirulo"),
			want:     "SELECT field1, field2, field3 FROM convenient_table JOIN another_convenient_table ON pirulo = $1 WHERE field1 > $2 AND field2 == $3 AND field3 > $4",
			wantArgs: []interface{}{"unpirulo", 1, 2, "pajarito"},
			wantErr:  false,
		},
		{
			name: "basic selection with where and join",
			chain: (&ExpresionChain{}).Delete("field1", "field2", "field3").
				Table("convenient_table").
				Where("field1 > ?", 1).
				Where("field2 == ?", 2).
				Where("field3 > ?", "pajarito").
				Join("another_convenient_table ON pirulo = ?", "unpirulo"),
			want:     "DELETE * FROM convenient_table JOIN another_convenient_table ON pirulo = $1 WHERE field1 > $2 AND field2 == $3 AND field3 > $4",
			wantArgs: []interface{}{"unpirulo", 1, 2, "pajarito"},
			wantErr:  false,
		},
		{
			name: "basic insert",
			chain: (&ExpresionChain{}).Insert(map[string]interface{}{"field1": "value1", "field2": 2, "field3": "blah"}).
				Table("convenient_table"),
			want:     "INSERT INTO $1 (field1, field2, field3) VALUES ($2, $3, $4)",
			wantArgs: []interface{}{"convenient_table", "value1", 2, "blah"},
			wantErr:  false,
		},
		{
			name: "basic selection with where and join",
			chain: (&ExpresionChain{}).Select("field1", "field2", "field3").
				Table("convenient_table").
				Where("field1 > ?", 1).
				Where("field2 == ?", 2).
				Where("field3 > ?", "pajarito").
				OrderBy("field2, field3").
				Join("another_convenient_table ON pirulo = ?", "unpirulo"),
			want:     "SELECT field1, field2, field3 FROM convenient_table JOIN another_convenient_table ON pirulo = $1 WHERE field1 > $2 AND field2 == $3 AND field3 > $4 ORDER BY field2, field3",
			wantArgs: []interface{}{"unpirulo", 1, 2, "pajarito"},
			wantErr:  false,
		},
		{
			name: "basic selection with where and join",
			chain: (&ExpresionChain{}).Select("field1", "field2", "field3").
				Table("convenient_table").
				Where("field1 > ?", 1).
				Where("field2 == ?", 2).
				Where("field3 > ?", "pajarito").
				GroupBy("field2, field3").
				Join("another_convenient_table ON pirulo = ?", "unpirulo"),
			want:     "SELECT field1, field2, field3 FROM convenient_table JOIN another_convenient_table ON pirulo = $1 WHERE field1 > $2 AND field2 == $3 AND field3 > $4 GROUP BY field2, field3",
			wantArgs: []interface{}{"unpirulo", 1, 2, "pajarito"},
			wantErr:  false,
		},
		{
			name: "basic selection with where and join",
			chain: (&ExpresionChain{}).Select("field1", "field2", "field3").
				Table("convenient_table").
				Where("field1 > ?", 1).
				Where("field2 == ?", 2).
				Where("field3 > ?", "pajarito").
				GroupBy("field2, field3").
				Limit(100).
				Offset(10).
				Join("another_convenient_table ON pirulo = ?", "unpirulo"),
			want:     "SELECT field1, field2, field3 FROM convenient_table JOIN another_convenient_table ON pirulo = $1 WHERE field1 > $2 AND field2 == $3 AND field3 > $4 GROUP BY field2, field3 LIMIT 100 OFFSET 10",
			wantArgs: []interface{}{"unpirulo", 1, 2, "pajarito"},
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec := tt.chain
			got, got1, err := ec.Render()
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpresionChain.Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpresionChain.Render() got = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.wantArgs) {
				t.Errorf("ExpresionChain.Render() got1 = %v, want %v", got1, tt.wantArgs)
			}
		})
	}
}
