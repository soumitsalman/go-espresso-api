package espressoapi_test

import (
	"context"
	"testing"

	"github.com/k0kubun/pp"
	"github.com/soumitsalman/espressoapi/cupboard"
	"github.com/stretchr/testify/assert"
)

var testCtx = context.Background()

func TestGetTags(t *testing.T) {
	db := setupTestDB()
	defer db.Close()
	page := cupboard.Pagination{Limit: 5, Offset: 10}
	tags, err := db.GetTags(context.Background(), page)
	assert.NoError(t, err)
	pp.Println("TAGS", tags)
}

func TestRelatedSips(t *testing.T) {
	db := setupTestDB()
	defer db.Close()
	cond := cupboard.Condition{
		IDs:          testRelatedIDs,
		Relationship: "SAME_AS",
	}
	page := cupboard.Pagination{}
	sips, err := db.QueryRelatedSips(context.Background(), cond, page)
	assert.NoError(t, err)
	assert.Greater(t, len(sips), 0)
	pp.Println("RELATED SIPS", sips)
}

func TestScalarSearchSips(t *testing.T) {
	db := setupTestDB()
	defer db.Close()
	cond := cupboard.Condition{
		Tags:    testScalarTags,
		Created: testSearchFrom(),
		Kinds:   cupboard.EVENTS,
	}
	page := cupboard.Pagination{}
	sips, err := db.QuerySips(context.Background(), cond, page)
	assert.NoError(t, err)
	assert.Greater(t, len(sips), 0)
	pp.Println("SIPS", sips)
}

func TestTextSearchSips(t *testing.T) {
	db := setupTestDB()
	defer db.Close()
	cond := cupboard.Condition{
		Tags:    testTextTags,
		FTS:     true,
		Created: testSearchFrom(),
	}
	page := cupboard.Pagination{}
	sips, err := db.QuerySips(context.Background(), cond, page)
	assert.NoError(t, err)
	assert.Greater(t, len(sips), 0)
	pp.Println("SIPS", sips)
}

func TestVectorSearchSips(t *testing.T) {
	db := setupTestDB()
	defer db.Close()
	cond := cupboard.Condition{
		Embedding: testQueryEmbedding,
		Distance:  0.4,
	}
	page := cupboard.Pagination{Limit: 5}
	sips, err := db.QuerySips(context.Background(), cond, page)
	assert.NoError(t, err)
	assert.Greater(t, len(sips), 0)
	pp.Println("SIPS", sips)
}
