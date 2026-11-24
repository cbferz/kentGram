#!/usr/bin/env python3
"""Executes every SQL statement in server/internal/store/sql.go against a real
PostgreSQL and asserts authorization, idempotency and limit behavior.

It creates a throw-away database (kg_sqlcheck_<pid>), applies migrations
0001..N, seeds three users and drops the database again. It never touches any
other database. Connection comes from the usual PG* environment variables
(PGHOST, PGPORT, PGUSER, PGPASSWORD); needs the `psql` client (>= 16 for \\bind).

    PGHOST=127.0.0.1 PGPORT=5432 PGUSER=postgres python3 server/scripts/check_message_sql.py
"""
import glob, json, os, re, subprocess, sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
DB = "kg_sqlcheck_%d" % os.getpid()
BASE = ["psql", "-X", "-q", "-v", "ON_ERROR_STOP=1"]

def psql(db, *args, input=None):
    return subprocess.run(BASE + ["-d", db] + list(args), input=input, capture_output=True, text=True)

def extract():
    src = open(os.path.join(ROOT, "internal/store/sql.go")).read()
    consts = {}
    for m in re.finditer(r"const (sql\w+) = (.*?)(?=\n// |\nconst |\Z)", src, re.S):
        body = m.group(2)
        refs = re.findall(r"\bsql\w+\b", body.split("`")[0]) if not body.strip().startswith("`") else []
        consts[m.group(1)] = (refs, re.findall(r"`(.*?)`", body, re.S))
    def build(n):
        refs, parts = consts[n]
        return (build(refs[0]) if refs else "") + "".join(parts)
    return {n: build(n) for n in consts}

Q = extract()

def setup():
    r = psql("postgres", "-c", "CREATE DATABASE " + DB)
    if r.returncode: sys.exit("cannot create temp database: " + r.stderr)
    for f in sorted(glob.glob(os.path.join(ROOT, "internal/store/migrations/*.sql"))):
        r = psql(DB, "-1", "-f", f)
        if r.returncode: teardown(); sys.exit("migration failed %s: %s" % (f, r.stderr))
    r = psql(DB, "-c", """
INSERT INTO users(username,email,password_hash) VALUES ('alice','a@x','h'),('bob','b@x','h'),('carol','c@x','h');
INSERT INTO chats(kind) VALUES ('direct'),('direct'),('direct');
INSERT INTO chat_members(chat_id,user_id) VALUES (1,1),(1,2),(2,1),(2,3),(3,2),(3,3);""")
    if r.returncode: teardown(); sys.exit("seed failed: " + r.stderr)

def teardown():
    psql("postgres", "-c", "DROP DATABASE IF EXISTS " + DB)

def run(name, *args):
    sql = " ".join(Q[name].split())
    bind = "".join(" '%s'" % str(a).replace("'", "''") for a in args)
    p = subprocess.run(BASE[:1] + ["-X", "-d", DB, "-At", "-F", "|", "-v", "ON_ERROR_STOP=1"], input=sql + "\n\\bind" + bind + " \\g\n", capture_output=True, text=True)
    if p.returncode != 0: return ("ERR", (p.stderr.strip().splitlines() or ["?"])[0])
    rows = [l for l in p.stdout.splitlines() if l and not l.startswith(("INSERT ", "UPDATE ", "DELETE "))]
    return ("OK", rows)

def main():
    global fails
    fails = 0
    def check(label,got,cond):
        global fails
        ok=cond(got)
        if not ok: fails+=1
        print(("PASS " if ok else "FAIL ")+label+("" if ok else "  -> got %r"%(got,)))
    def ids(r): return r[1] if r[0]=="OK" else None
    # --- send / idempotency / authorization
    r=run("sqlSendMessage",1,1,"hello",0,"");               check("send by member returns id",r,lambda r:r[0]=="OK" and len(r[1])==1)
    m1=int(r[1][0])
    r=run("sqlSendMessage",1,2,"hi back",m1,"k-1");         check("reply in same chat ok",r,lambda r:r[0]=="OK" and len(r[1])==1)
    m2=int(r[1][0])
    r=run("sqlSendMessage",1,2,"hi back",m1,"k-1");         check("replayed client_id inserts nothing",r,lambda r:r==("OK",[]))
    r=run("sqlMessageByClientID",2,"k-1");                  check("replay lookup finds original",r,lambda r:r==("OK",["%d|1"%m2]))
    r=run("sqlSendMessage",1,3,"intruder",0,"");            check("non-member cannot send",r,lambda r:r==("OK",[]))
    r=run("sqlSendMessage",2,1,"x",m1,"");                  check("reply cannot cross chats",r,lambda r:r==("OK",[]))
    # --- read views
    r=run("sqlMessageByID",1,2,m2);                         check("byID has 15 cols, empty reactions, reply fields",r,lambda r:r[0]=="OK" and len(r[1])==1 and len(r[1][0].split("|"))==15 and r[1][0].endswith("|[]"))
    r=run("sqlMessageHistory",1,1,0,50);                    check("history returns both, newest first",r,lambda r:r[0]=="OK" and [x.split("|")[0] for x in r[1]]==[str(m2),str(m1)])
    r=run("sqlMessageByID",2,1,m1);                         check("byID with wrong chat hides message",r,lambda r:r==("OK",[]))
    # --- edit
    r=run("sqlEditMessage",m1,1,1,"hello (edited)");        check("sender can edit",r,lambda r:r==("OK",[str(m1)]))
    r=run("sqlEditMessage",m1,1,2,"hijack");                check("other member cannot edit",r,lambda r:r==("OK",[]))
    r=run("sqlEditMessage",m1,2,1,"wrong chat");            check("edit with wrong chat id refused",r,lambda r:r==("OK",[]))
    r=run("sqlEditMessage",m1,1,3,"outsider");              check("non-member cannot edit",r,lambda r:r==("OK",[]))
    # --- reactions
    r=run("sqlLockMessageForReaction",m1,1,2);              check("member may react (lock row)",r,lambda r:r==("OK",["1"]))
    r=run("sqlLockMessageForReaction",m1,1,3);              check("non-member cannot react",r,lambda r:r==("OK",[]))
    r=run("sqlLockMessageForReaction",m1,2,1);              check("wrong chat cannot react",r,lambda r:r==("OK",[]))
    for e in ["👍","❤️","😂","🔥"]:
        run("sqlAddReaction",m1,1,2,e); psql(DB, "-At", "-c", "SELECT pg_sleep(0.01)")
    run("sqlEvictOldReactions",m1,2,3)
    r=run("sqlReactionTotals",m1,2);                        check("user keeps newest 3 of 4 (oldest evicted)",r,lambda r:r[0]=="OK" and [x.split("|")[0] for x in r[1]]==["❤️","😂","🔥"])
    r=run("sqlCountReactionTypes",m1);                      check("distinct types counted",r,lambda r:r==("OK",["3"]))
    run("sqlRemoveReaction",m1,2,"🔥")
    r=run("sqlReactionTotals",m1,2);                        check("remove drops only that emoji",r,lambda r:r[0]=="OK" and len(r[1])==2)
    r=run("sqlMessageByID",1,2,m1);                         check("reactions aggregated into message JSON with mine flag",r,lambda r:r[0]=="OK" and '"mine": true' in r[1][0] and '"count": 1' in r[1][0])
    r=run("sqlMessageByID",1,1,m1);                         check("other viewer sees mine=false",r,lambda r:r[0]=="OK" and '"mine": false' in r[1][0])
    # --- pins
    r=run("sqlPinMessage",1,m1,2);                          check("member pins -> visible=t",r,lambda r:r==("OK",["t"]))
    r=run("sqlPinMessage",1,m1,2);                          check("pin is idempotent",r,lambda r:r==("OK",["t"]))
    r=run("sqlPinMessage",1,m1,3);                          check("non-member pin -> visible=f",r,lambda r:r==("OK",["f"]))
    r=run("sqlPinMessage",2,m1,1);                          check("pin with wrong chat -> visible=f",r,lambda r:r==("OK",["f"]))
    r=run("sqlPinnedMessages",1,1);                         check("pinned list returns the pin",r,lambda r:r[0]=="OK" and len(r[1])==1 and r[1][0].split("|")[0]==str(m1))
    r=run("sqlMessageByID",1,1,m1);                         check("message reports pinned=t",r,lambda r:r[0]=="OK" and "|t|" in r[1][0])
    r=run("sqlUnpinMessage",1,m1,3);                        check("non-member unpin -> member=f",r,lambda r:r==("OK",["f"]))
    r=run("sqlPinnedMessages",1,1);                         check("outsider unpin did not remove pin",r,lambda r:r[0]=="OK" and len(r[1])==1)
    r=run("sqlUnpinMessage",1,m1,1);                        check("member unpin ok",r,lambda r:r==("OK",["t"]))
    r=run("sqlPinnedMessages",1,1);                         check("pin removed",r,lambda r:r==("OK",[]))
    # --- forward   (args: src_chat, message, user, dst_chat, client_id)
    r=run("sqlForwardMessage",1,m1,1,3,"");                 check("alice cannot forward into chat3 (she is not in it)",r,lambda r:r==("OK",[]))
    r=run("sqlForwardMessage",1,m1,3,2,"");                 check("carol cannot forward out of chat1 (not a member of source)",r,lambda r:r==("OK",[]))
    r=run("sqlForwardMessage",2,m1,1,1,"");                 check("forward with wrong source chat refused",r,lambda r:r==("OK",[]))
    r=run("sqlForwardMessage",1,m2,1,2,"fw-1");             check("alice (member of both) forwards bob's message into chat2",r,lambda r:r[0]=="OK" and len(r[1])==1)
    f1=int(r[1][0])
    r=run("sqlMessageByID",2,1,f1);                         check("copy lives in chat2, sent by alice, author preserved = bob",r,lambda r:r[0]=="OK" and len(r[1])==1 and r[1][0].split("|")[1]=="2" and r[1][0].split("|")[3]=="alice" and r[1][0].split("|")[11]=="2" and r[1][0].split("|")[12]=="bob")
    r=run("sqlForwardMessage",1,m2,1,2,"fw-1");             check("forward replay inserts nothing",r,lambda r:r==("OK",[]))
    r=run("sqlMessageByClientID",1,"fw-1");                 check("replay lookup resolves to chat2 copy",r,lambda r:r==("OK",["%d|2"%f1]))
    r=run("sqlForwardMessage",2,f1,1,1,"");                 check("forward-of-forward into chat1",r,lambda r:r[0]=="OK" and len(r[1])==1)
    f2=int(r[1][0]); r=run("sqlMessageByID",1,1,f2);        check("  ...author is still bob, not alice",r,lambda r:r[0]=="OK" and r[1][0].split("|")[12]=="bob")
    # --- read markers
    r=run("sqlMarkRead",1,1,999999);                        check("marker clamps to newest message id of chat",r,lambda r:r[0]=="OK" and int(r[1][0])<=f2 and int(r[1][0])>=m2)
    hi=int(r[1][0]); r=run("sqlMarkRead",1,1,1);            check("marker never moves backwards",r,lambda r:r==("OK",[str(hi)]))
    r=run("sqlMarkRead",1,3,5);                             check("non-member mark-read changes nothing",r,lambda r:r==("OK",[]))
    r=run("sqlPeerLastRead",1,2);                           check("peer last read visible to other member",r,lambda r:r==("OK",[str(hi)]))
    # --- delete (and pin cleanup)
    run("sqlPinMessage",1,m1,1)
    r=run("sqlDeleteMessage",m1,1,2);                       check("non-sender cannot delete",r,lambda r:r==("OK",[]))
    r=run("sqlDeleteMessage",m1,2,1);                       check("delete with wrong chat refused",r,lambda r:r==("OK",[]))
    r=run("sqlDeleteMessage",m1,1,1);                       check("sender deletes",r,lambda r:r==("OK",[str(m1)]))
    r=run("sqlDeleteMessage",m1,1,1);                       check("second delete -> not found",r,lambda r:r==("OK",[]))
    r=run("sqlPinnedMessages",1,1);                         check("deleting a message unpins it",r,lambda r:r==("OK",[]))
    r=run("sqlMessageByID",1,1,m1);                         check("deleted message no longer readable",r,lambda r:r==("OK",[]))
    r=run("sqlEditMessage",m1,1,1,"zombie");                check("deleted message cannot be edited",r,lambda r:r==("OK",[]))
    r=run("sqlChatMemberIDs",1);                            check("chat members",r,lambda r:r[0]=="OK" and sorted(r[1])==["1","2"])
    r=run("sqlPeerIDs",1);                                  check("peer ids of alice = bob+carol",r,lambda r:r[0]=="OK" and sorted(r[1])==["2","3"])
    print("\nFAILURES:", fails); return 1 if fails else 0


if __name__ == "__main__":
    setup()
    try:
        code = main()
    finally:
        teardown()
    sys.exit(code)
