package directory

import (
	"context"
	"fmt"
	"github.com/srabraham/ranger-ims-go/conf"
	clubhousequeries "github.com/srabraham/ranger-ims-go/directory/clubhousedb"
	imsjson "github.com/srabraham/ranger-ims-go/json"
	"slices"
)

type UserStore struct {
	testUsers   []conf.TestUser
	clubhouseDB *DB
}

func NewUserStore(testUsers []conf.TestUser, clubhouseDB *DB) (*UserStore, error) {
	if clubhouseDB == nil && testUsers == nil {
		return nil, fmt.Errorf("NewUserStore: one of clubhouseDB or testUsers must be provided")
	}
	if clubhouseDB != nil && testUsers != nil {
		return nil, fmt.Errorf("NewUserStore: only one of clubhouseDB or testUsers may be provided")
	}
	return &UserStore{
		testUsers:   testUsers,
		clubhouseDB: clubhouseDB,
	}, nil
}

func (users UserStore) GetRangers(ctx context.Context) ([]imsjson.Person, error) {
	var response []imsjson.Person

	if users.testUsers != nil {
		for _, user := range users.testUsers {
			response = append(response, imsjson.Person{
				Handle:      user.Handle,
				Status:      user.Status,
				Onsite:      user.Onsite,
				DirectoryID: user.DirectoryID,
			})
		}
		return response, nil
	}

	results, err := clubhousequeries.New(users.clubhouseDB).RangersById(ctx)
	if err != nil {
		return nil, fmt.Errorf("[RangersById] %w", err)
	}

	for _, r := range results {
		response = append(response, imsjson.Person{
			Handle:      r.Callsign,
			Status:      string(r.Status),
			Onsite:      r.OnSite,
			DirectoryID: r.ID,
		})
	}

	return response, nil
}

func (users UserStore) GetUserPositionsTeams(ctx context.Context, userID int64) (positions, teams []string, err error) {
	if users.testUsers != nil {
		for _, user := range users.testUsers {
			if user.DirectoryID == userID {
				positions = append(positions, user.Positions...)
				teams = append(teams, user.Teams...)
				break
			}
		}
		return positions, teams, nil
	}

	teamRows, err := clubhousequeries.New(users.clubhouseDB).Teams(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[Teams]: %w", err)
	}
	positionRows, err := clubhousequeries.New(users.clubhouseDB).Positions(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[Positions]: %w", err)
	}
	personTeams, err := clubhousequeries.New(users.clubhouseDB).PersonTeams(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[PersonTeams]: %w", err)
	}
	personPositions, err := clubhousequeries.New(users.clubhouseDB).PersonPositions(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[PersonPositions]: %w", err)
	}

	var foundPositions []uint64
	var foundPositionNames []string
	var foundTeams []int32
	var foundTeamNames []string
	for _, pp := range personPositions {
		if pp.PersonID == uint64(userID) {
			foundPositions = append(foundPositions, pp.PositionID)
		}
	}
	for _, pos := range positionRows {
		if slices.Contains(foundPositions, pos.ID) {
			foundPositionNames = append(foundPositionNames, pos.Title)
		}
	}
	for _, pt := range personTeams {
		if pt.PersonID == int32(userID) {
			foundTeams = append(foundTeams, pt.TeamID)
		}
	}
	for _, team := range teamRows {
		if slices.Contains(foundTeams, int32(team.ID)) {
			foundTeamNames = append(foundTeamNames, team.Title)
		}
	}
	return foundPositionNames, foundTeamNames, nil
}
