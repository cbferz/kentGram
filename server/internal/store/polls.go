package store

import (
	"context"
	"strings"
	"time"

)

type PollOption struct {
	ID         int64  `json:"id"`
	Text       string `json:"text"`
	Votes      int64  `json:"votes"`
	Selected   bool   `json:"selected"`
}

type Poll struct {
	ID             int64        `json:"id"`
	ChatID         int64        `json:"chat_id"`
	CreatorID      int64        `json:"creator_id"`
	Question       string       `json:"question"`
	MultipleChoice bool         `json:"multiple_choice"`
	Anonymous      bool         `json:"anonymous"`
	Closed         bool         `json:"closed"`
	TotalVotes     int64        `json:"total_votes"`
	CreatedAt      time.Time    `json:"created_at"`
	Options        []PollOption `json:"options"`
}

func (s *Store) CreatePoll(ctx context.Context, chatID, creatorID int64, question string, options []string, multipleChoice, anonymous bool) (Poll, error) {
	var p Poll
	tx, err := s.Pool.Begin(ctx)
	if err != nil { return p, err }
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `INSERT INTO polls(chat_id,creator_id,question,multiple_choice,anonymous)
		SELECT $1,$2,$3,$4,$5
		WHERE EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)
		  AND EXISTS(SELECT 1 FROM chats c WHERE c.id=$1 AND
		    (c.kind <> 'channel' OR EXISTS(
		      SELECT 1 FROM chat_members cm WHERE cm.chat_id=c.id AND cm.user_id=$2 AND cm.role IN ('owner','admin')
		    )))
		RETURNING id,chat_id,creator_id,question,multiple_choice,anonymous,is_closed,created_at`,
		chatID, creatorID, strings.TrimSpace(question), multipleChoice, anonymous,
	).Scan(&p.ID,&p.ChatID,&p.CreatorID,&p.Question,&p.MultipleChoice,&p.Anonymous,&p.Closed,&p.CreatedAt)
	if err != nil { return p, norm(err) }

	for i, option := range options {
		if _, err = tx.Exec(ctx, `INSERT INTO poll_options(poll_id,option_text,position) VALUES($1,$2,$3)`,
			p.ID, strings.TrimSpace(option), i); err != nil { return p, norm(err) }
	}
	if err = tx.Commit(ctx); err != nil { return Poll{}, err }
	return s.GetPoll(ctx, chatID, p.ID, creatorID)
}

func (s *Store) GetPoll(ctx context.Context, chatID, pollID, viewerID int64) (Poll, error) {
	var p Poll
	err := s.Pool.QueryRow(ctx, `SELECT p.id,p.chat_id,p.creator_id,p.question,p.multiple_choice,p.anonymous,p.is_closed,p.created_at,
		(SELECT count(DISTINCT user_id) FROM poll_votes WHERE poll_id=p.id)
		FROM polls p JOIN chat_members cm ON cm.chat_id=p.chat_id AND cm.user_id=$3
		WHERE p.chat_id=$1 AND p.id=$2`, chatID, pollID, viewerID).
		Scan(&p.ID,&p.ChatID,&p.CreatorID,&p.Question,&p.MultipleChoice,&p.Anonymous,&p.Closed,&p.CreatedAt,&p.TotalVotes)
	if err != nil { return p, norm(err) }

	rows, err := s.Pool.Query(ctx, `SELECT o.id,o.option_text,count(v.user_id),
		EXISTS(SELECT 1 FROM poll_votes mine WHERE mine.poll_id=o.poll_id AND mine.option_id=o.id AND mine.user_id=$2)
		FROM poll_options o LEFT JOIN poll_votes v ON v.poll_id=o.poll_id AND v.option_id=o.id
		WHERE o.poll_id=$1 GROUP BY o.id,o.option_text,o.position ORDER BY o.position`, pollID, viewerID)
	if err != nil { return p, err }
	defer rows.Close()
	p.Options = []PollOption{}
	for rows.Next() {
		var option PollOption
		if err := rows.Scan(&option.ID,&option.Text,&option.Votes,&option.Selected); err != nil { return p, err }
		p.Options = append(p.Options, option)
	}
	return p, rows.Err()
}

func (s *Store) VotePoll(ctx context.Context, chatID, pollID, userID int64, optionIDs []int64) (Poll, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil { return Poll{}, err }
	defer tx.Rollback(ctx)

	var multiple, closed bool
	err = tx.QueryRow(ctx, `SELECT p.multiple_choice,p.is_closed
		FROM polls p JOIN chat_members cm ON cm.chat_id=p.chat_id AND cm.user_id=$3
		WHERE p.chat_id=$1 AND p.id=$2 FOR UPDATE OF p`, chatID, pollID, userID).Scan(&multiple,&closed)
	if err != nil { return Poll{}, norm(err) }
	if closed { return Poll{}, ErrConflict }
	if len(optionIDs)==0 || (!multiple && len(optionIDs)!=1) { return Poll{}, ErrConflict }

	seen := make(map[int64]struct{},len(optionIDs))
	for _, optionID := range optionIDs {
		if optionID<=0 { return Poll{}, ErrNotFound }
		if _, ok := seen[optionID]; ok { return Poll{}, ErrConflict }
		seen[optionID]=struct{}{}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM poll_options WHERE poll_id=$1 AND id=$2)`,pollID,optionID).Scan(&exists); err != nil { return Poll{}, err }
		if !exists { return Poll{}, ErrNotFound }
	}
	if !multiple {
		if _, err := tx.Exec(ctx,`DELETE FROM poll_votes WHERE poll_id=$1 AND user_id=$2`,pollID,userID); err != nil { return Poll{},err }
	}
	for _, optionID := range optionIDs {
		if _, err := tx.Exec(ctx,`INSERT INTO poll_votes(poll_id,option_id,user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,pollID,optionID,userID); err != nil { return Poll{},err }
	}
	if err := tx.Commit(ctx); err != nil { return Poll{},err }
	return s.GetPoll(ctx,chatID,pollID,userID)
}

func (s *Store) ClosePoll(ctx context.Context, chatID, pollID, actorID int64) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE polls p SET is_closed=TRUE
		WHERE p.id=$2 AND p.chat_id=$1 AND p.is_closed=FALSE
		  AND (p.creator_id=$3 OR EXISTS(
		    SELECT 1 FROM chat_members cm WHERE cm.chat_id=p.chat_id AND cm.user_id=$3 AND cm.role IN ('owner','admin')
		  ))
		  AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=p.chat_id AND user_id=$3)`,
		chatID,pollID,actorID)
	if err != nil { return err }
	if tag.RowsAffected()==0 { return ErrNotFound }
	return nil
}


func (s *Store) ListPolls(ctx context.Context, chatID, viewerID int64, limit int) ([]Poll, error) {
	member, err := s.IsChatMember(ctx,chatID,viewerID)
	if err != nil { return nil,err }
	if !member { return nil,ErrNotFound }
	if limit < 1 || limit > 100 { limit = 50 }
	rows, err := s.Pool.Query(ctx, `SELECT p.id FROM polls p
		JOIN chat_members cm ON cm.chat_id=p.chat_id AND cm.user_id=$2
		WHERE p.chat_id=$1 ORDER BY p.created_at DESC,p.id DESC LIMIT $3`, chatID, viewerID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil { return nil, err }
		ids = append(ids,id)
	}
	if err := rows.Err(); err != nil { return nil, err }
	out := make([]Poll,0,len(ids))
	for _, id := range ids {
		p, err := s.GetPoll(ctx,chatID,id,viewerID)
		if err != nil { return nil,err }
		out=append(out,p)
	}
	return out,nil
}
