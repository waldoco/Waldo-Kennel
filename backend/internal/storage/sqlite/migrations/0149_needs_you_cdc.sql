-- +goose Up
-- +goose StatementBegin
CREATE TRIGGER owner_answer_questions_needs_you_insert AFTER INSERT ON owner_answer_questions BEGIN
 INSERT INTO change_log(project_id,session_id,event_type,payload,created_at)
 SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('questionId',NEW.id,'generation',NEW.generation,'status','open')),NEW.updated_at
 FROM conversations c JOIN sessions s ON s.id=c.current_session_id WHERE c.id=NEW.conversation_id;
END;
CREATE TRIGGER owner_answer_questions_needs_you_update AFTER UPDATE ON owner_answer_questions WHEN OLD.status<>NEW.status BEGIN
 INSERT INTO change_log(project_id,session_id,event_type,payload,created_at)
 SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('questionId',NEW.id,'generation',NEW.generation,'status',NEW.status)),NEW.updated_at
 FROM conversations c JOIN sessions s ON s.id=c.current_session_id WHERE c.id=NEW.conversation_id;
END;
CREATE TRIGGER governed_controls_needs_you_insert AFTER INSERT ON governed_control_commands WHEN NEW.command_class='answer' BEGIN
 INSERT INTO change_log(project_id,session_id,event_type,payload,created_at)
 SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('generation',NEW.request_instance_id,'commandId',NEW.id,'status',NEW.state)),NEW.updated_at FROM sessions s WHERE s.id=NEW.session_id;
END;
CREATE TRIGGER governed_controls_needs_you_update AFTER UPDATE ON governed_control_commands WHEN NEW.command_class='answer' AND OLD.state<>NEW.state BEGIN
 INSERT INTO change_log(project_id,session_id,event_type,payload,created_at)
 SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('generation',NEW.request_instance_id,'commandId',NEW.id,'status',NEW.state)),NEW.updated_at FROM sessions s WHERE s.id=NEW.session_id;
END;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS governed_controls_needs_you_update;
DROP TRIGGER IF EXISTS governed_controls_needs_you_insert;
DROP TRIGGER IF EXISTS owner_answer_questions_needs_you_update;
DROP TRIGGER IF EXISTS owner_answer_questions_needs_you_insert;
-- +goose StatementEnd
