package com.kentgram;

import android.app.Activity;
import android.graphics.Color;
import android.graphics.Typeface;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.widget.*;
import android.content.SharedPreferences;
import android.content.Intent;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.ArrayDeque;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Set;

public final class MainActivity extends Activity {
    private static final String API="https://api.kentgram.tendercheck.pro/api/v1";
    private static final int BG=0xFF0D1118,PANEL=0xFF151C27,TEXT=0xFFF4F7FB,MUTED=0xFF9AA9BC;
    private final ExecutorService io=Executors.newSingleThreadExecutor();
    private SharedPreferences prefs;
    private volatile String token=""; // read from the realtime thread too
    private LinearLayout root,content,top,navBar;
    private EditText composer;
    private LinearLayout replyPreview;
    private long replyToMessageId=0;
    private String replyToBody="", replyToSender="";
    private long currentChat=0;
    // Views of the screen currently shown; null when that screen is not on top. Used by realtime sync.
    private LinearLayout openMessages,openChatsList;
    private ScrollView openHistory;
    private RealtimeClient realtime;
    private final Handler ui=new Handler(Looper.getMainLooper());
    private Runnable pendingSync;
    private final HashMap<Long,ArrayDeque<String>> sendQueues=new HashMap<>();
    private final HashMap<Long,ArrayDeque<Long>> replyQueues=new HashMap<>();
    private final HashSet<Long> sendingChats=new HashSet<>();

    @Override public void onCreate(Bundle state){super.onCreate(state);getWindow().setStatusBarColor(Color.BLACK);getWindow().setNavigationBarColor(BG);prefs=getSharedPreferences("kentgram",MODE_PRIVATE);token=prefs.getString("access_token","");if(token.isEmpty())showAuth();else showChats();}

    private void base(String title){
        openMessages=null;openHistory=null;openChatsList=null;
        root=new LinearLayout(this);root.setOrientation(1);root.setBackgroundColor(BG);
        top=new LinearLayout(this);top.setGravity(Gravity.CENTER_VERTICAL);top.setPadding(dp(18),dp(12),dp(18),dp(12));top.setBackground(round(PANEL,22));
        TextView h=label(title,21,TEXT,true);top.addView(h,new LinearLayout.LayoutParams(0,-2,1));LinearLayout.LayoutParams headerParams=new LinearLayout.LayoutParams(-1,-2);headerParams.setMargins(dp(10),dp(8),dp(10),dp(4));root.addView(top,headerParams);
        ScrollView scroll=new ScrollView(this);content=new LinearLayout(this);content.setOrientation(1);content.setPadding(dp(16),dp(16),dp(16),dp(16));scroll.addView(content);root.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));
        buildNavigation(title);
        setContentView(root);
    }

    private void buildNavigation(String title){
        navBar=new LinearLayout(this);
        navBar.setGravity(Gravity.CENTER);
        navBar.setOrientation(LinearLayout.HORIZONTAL);
        navBar.setPadding(dp(5),dp(5),dp(5),dp(5));
        navBar.setBackground(round(0xFF202124,32));

        String[] labels={"Чаты","Контакты","Лента","Настройки","Профиль"};
        String[] icons={"▤","♧","▧","⚙","●"};
        String[] screens={"chats","contacts","feed","settings","profile"};
        String active=title.equals("kentGram")?"chats":title.equals("Контакты")?"contacts":
                title.equals("Лента")?"feed":title.equals("Настройки")?"settings":"";

        for(int i=0;i<labels.length;i++){
            final String screen=screens[i];
            boolean selected=screen.equals(active);
            LinearLayout item=new LinearLayout(this);
            item.setGravity(Gravity.CENTER);
            item.setOrientation(1);
            item.setPadding(dp(1),dp(5),dp(1),dp(5));
            item.setBackground(round(selected?0xFF293246:Color.TRANSPARENT,26));

            TextView icon=label(icons[i],22,selected?0xFF5685FF:0xFFE4E8F0,true);
            icon.setGravity(Gravity.CENTER);
            TextView text=label(labels[i],10,selected?0xFF5685FF:0xFFE4E8F0,true);
            text.setGravity(Gravity.CENTER);
            text.setMaxLines(1);
            text.setIncludeFontPadding(false);
            item.addView(icon);
            item.addView(text);

            LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(0,dp(58),1);
            lp.setMargins(dp(1),0,dp(1),0);
            navBar.addView(item,lp);
            item.setContentDescription(labels[i]);
            item.setOnClickListener(v->navigate(screen));
        }
        LinearLayout.LayoutParams navParams=new LinearLayout.LayoutParams(-1,-2);
        navParams.setMargins(dp(10),dp(4),dp(10),dp(7));
        root.addView(navBar,navParams);
    }
    private void navigate(String screen){
        switch(screen){
            case "chats":showChats();break;
            case "contacts":showContacts();break;
            case "feed":showFeed();break;
            case "settings":showSettings();break;
            case "profile":startActivity(new Intent(this,ProfileActivity.class));break;
        }
    }
    private void showContacts(){
        base("Контакты");
        TextView heading=label("Мои контакты",17,TEXT,true);content.addView(heading);
        LinearLayout contactsList=new LinearLayout(this);contactsList.setOrientation(1);content.addView(contactsList);
        requestGet("/contacts",r->{
            contactsList.removeAllViews();JSONArray contacts=r.optJSONArray("contacts");
            if(contacts==null||contacts.length()==0){contactsList.addView(label("Пока нет контактов. Найди пользователя ниже и открой его профиль, чтобы подписаться.",13,MUTED,false));}
            else for(int i=0;i<contacts.length();i++){
                JSONObject u=contacts.optJSONObject(i);if(u==null)continue;
                String username=u.optString("username"),name=u.optString("display_name");
                LinearLayout card=new LinearLayout(this);card.setOrientation(1);card.setPadding(dp(13),dp(10),dp(13),dp(10));card.setBackground(round(PANEL,13));
                card.addView(label(name.isEmpty()?"@"+username:name,15,TEXT,true));
                card.addView(label("@"+username,12,MUTED,false));
                Button open=button("Профиль");card.addView(open);
                open.setOnClickListener(v->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(7);contactsList.addView(card,lp);
            }
        });
        TextView discover=label("Найти людей",17,TEXT,true);discover.setPadding(0,dp(18),0,dp(8));content.addView(discover);
        EditText query=field("Имя или username");content.addView(query);
        Button search=button("Найти пользователей");content.addView(search);
        LinearLayout results=new LinearLayout(this);results.setOrientation(1);content.addView(results);
        search.setOnClickListener(v->{
            String q=query.getText().toString().trim();if(q.length()<2){toast("Введите минимум 2 символа");return;}
            search.setEnabled(false);
            requestGet("/users/search?q="+enc(q),r->{
                search.setEnabled(true);results.removeAllViews();JSONArray users=r.optJSONArray("users");
                if(users==null||users.length()==0){results.addView(label("Ничего не найдено",14,MUTED,false));return;}
                for(int i=0;i<users.length();i++){
                    JSONObject u=users.optJSONObject(i);if(u==null)continue;
                    String username=u.optString("username"),name=u.optString("display_name");
                    LinearLayout card=new LinearLayout(this);card.setOrientation(1);card.setPadding(dp(13),dp(11),dp(13),dp(11));card.setBackground(round(PANEL,14));
                    card.addView(label(name.isEmpty()?"@"+username:name,16,TEXT,true));
                    card.addView(label("@"+username,13,MUTED,false));
                    Button open=button("Открыть профиль");card.addView(open);
                    open.setOnClickListener(x->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
                    LinearLayout.LayoutParams cp=new LinearLayout.LayoutParams(-1,-2);cp.bottomMargin=dp(8);results.addView(card,cp);
                }
            });
        });
    }
    private void showFeed(){
        base("Лента");
        content.addView(label("Публикации появятся следующим этапом",17,TEXT,true));
        content.addView(label("Сейчас готовим API публикаций, реакции и комментарии. Пока здесь нет демонстрационных или выдуманных постов.",14,MUTED,false));
    }
    private void showSettings(){
        base("Настройки");
        content.addView(label("Аккаунт",13,MUTED,true));
        Button profile=button("Мой профиль");content.addView(profile);profile.setOnClickListener(v->startActivity(new Intent(this,ProfileActivity.class)));
        Button logout=button("Выйти из аккаунта");content.addView(logout);
        logout.setOnClickListener(v->{request("/auth/logout",new JSONObject(),true,r->{});prefs.edit().clear().apply();token="";showAuth();});
    }

    private void showAuth(){
        stopRealtime();
        base("kentGram");
        if(navBar!=null)root.removeView(navBar);
        content.setPadding(dp(20),dp(24),dp(20),dp(28));

        LinearLayout hero=new LinearLayout(this);
        hero.setOrientation(1);
        hero.setGravity(Gravity.CENTER);
        hero.setPadding(dp(20),dp(26),dp(20),dp(24));
        hero.setBackground(round(PANEL,26));
        TextView mark=label("k",38,0xFF76A7FF,true);
        mark.setGravity(Gravity.CENTER);
        mark.setBackground(round(0xFF203A61,28));
        LinearLayout.LayoutParams mp=new LinearLayout.LayoutParams(dp(76),dp(76));
        mp.bottomMargin=dp(16);
        hero.addView(mark,mp);
        TextView title=label("Твой круг общения",23,TEXT,true);
        title.setGravity(Gravity.CENTER);
        hero.addView(title);
        TextView subtitle=label("Общайся. Находи своих. Оставайся на связи.",13,MUTED,false);
        subtitle.setGravity(Gravity.CENTER);
        subtitle.setPadding(0,dp(8),0,dp(2));
        hero.addView(subtitle);
        content.addView(hero);

        LinearLayout form=new LinearLayout(this);
        form.setOrientation(1);
        form.setPadding(dp(18),dp(20),dp(18),dp(18));
        form.setBackground(round(0xFF111925,24));
        LinearLayout.LayoutParams fp=new LinearLayout.LayoutParams(-1,-2);
        fp.topMargin=dp(16);
        content.addView(form,fp);

        TextView formTitle=label("Вход в аккаунт",18,TEXT,true);
        formTitle.setPadding(0,0,0,dp(14));
        form.addView(formTitle);
        EditText email=field("Email");
        EditText password=field("Пароль");
        password.setInputType(129);
        EditText username=field("Имя пользователя");
        username.setVisibility(View.GONE);
        form.addView(email);
        form.addView(password);
        form.addView(username);

        Button primary=button("Войти");
        Button secondary=button("Создать аккаунт");
        primary.setTextSize(16);
        primary.setMinHeight(dp(60));
        primary.setBackground(round(0xFF2878C7,40));
        secondary.setTextSize(14);
        secondary.setMinHeight(dp(48));
        secondary.setBackground(round(0xFF202A38,40));
        form.addView(primary);
        LinearLayout.LayoutParams secondaryParams=(LinearLayout.LayoutParams)secondary.getLayoutParams();
        if(secondaryParams==null)secondaryParams=new LinearLayout.LayoutParams(-1,-2);
        secondaryParams.topMargin=dp(4);
        secondary.setLayoutParams(secondaryParams);
        form.addView(secondary);

        final boolean[] registering={false};
        primary.setOnClickListener(v->{
            String mail=email.getText().toString().trim();
            String pass=password.getText().toString();
            if(mail.isEmpty()||pass.isEmpty()){
                KentDialog.showMessage(this,"Заполни поля","Укажи email и пароль, чтобы продолжить.","Понятно",null);
                return;
            }
            if(registering[0]&&username.getText().toString().trim().isEmpty()){
                KentDialog.showMessage(this,"Нужно имя пользователя","Придумай username для нового аккаунта.","Понятно",null);
                return;
            }
            auth(mail,pass,registering[0]?username.getText().toString():null);
        });
        secondary.setOnClickListener(v->{
            registering[0]=!registering[0];
            username.setVisibility(registering[0]?View.VISIBLE:View.GONE);
            formTitle.setText(registering[0]?"Создание аккаунта":"Вход в аккаунт");
            primary.setText(registering[0]?"Зарегистрироваться":"Войти");
            secondary.setText(registering[0]?"Уже есть аккаунт? Войти":"Создать аккаунт");
        });
    }
    private void auth(String email,String password,String username){
        JSONObject b=new JSONObject();try{b.put("email",email.trim());b.put("password",password);b.put("device_name","kentGram Android");if(username!=null)b.put("username",username.trim());}catch(Exception ignored){}
        request(username==null?"/auth/login":"/auth/register",b,false,r->{token=r.optString("access_token","");if(token.isEmpty()){toast("Сервер не вернул токен");return;}prefs.edit().putString("access_token",token).putString("refresh_token",r.optString("refresh_token","")).apply();showChats();});
    }
    private void showChats(){
        currentChat=0;
        base("kentGram");
        ensureRealtime();

        TextView searchTitle=label("Поиск людей",16,TEXT,true);
        content.addView(searchTitle);
        EditText query=field("Имя или @username");
        content.addView(query);
        Button search=button("Найти");
        content.addView(search);

        LinearLayout results=new LinearLayout(this);
        results.setOrientation(1);
        content.addView(results);

        TextView caption=label("Мои переписки",16,TEXT,true);
        caption.setPadding(0,dp(14),0,dp(9));
        content.addView(caption);
        LinearLayout chatsList=new LinearLayout(this);
        chatsList.setOrientation(1);
        content.addView(chatsList);
        openChatsList=chatsList;
        loadChats(chatsList);

        search.setOnClickListener(v->searchUsers(query,search,results));
        query.setOnEditorActionListener((v,action,event)->{
            searchUsers(query,search,results);
            return true;
        });

        // Floating action button sits above the bottom navigation.
        LinearLayout fabBar=new LinearLayout(this);
        fabBar.setGravity(Gravity.RIGHT|Gravity.CENTER_VERTICAL);
        fabBar.setPadding(dp(16),dp(2),dp(18),dp(10));
        TextView fab=label("+",30,Color.WHITE,true);
        fab.setGravity(Gravity.CENTER);
        fab.setBackground(round(0xFF2878C7,50));
        fab.setElevation(dp(7));
        fab.setContentDescription("Новая переписка");
        fab.setOnClickListener(v->showUserSearch());
        fabBar.addView(fab,new LinearLayout.LayoutParams(dp(58),dp(58)));
        int navIndex=root.indexOfChild(navBar);
        root.addView(fabBar,navIndex<0?root.getChildCount():navIndex,
                new LinearLayout.LayoutParams(-1,-2));
    }

    private void loadChats(LinearLayout chatsList){loadChats(chatsList,false);}
    private void loadChats(LinearLayout chatsList,boolean quiet){
        request("/chats",null,true,r->{
            chatsList.removeAllViews();
            JSONArray a=r.optJSONArray("chats");
            if(a==null||a.length()==0){
                chatsList.addView(label("Пока нет переписок. Начни новую через кнопку +.",14,MUTED,false));
                return;
            }
            for(int i=0;i<a.length();i++){
                JSONObject c=a.optJSONObject(i);
                if(c==null)continue;
                long id=c.optLong("id");
                String name=c.optString("peer_display_name");
                if(name.isEmpty())name="@"+c.optString("peer_username");
                String last=c.optString("last_message","Начните общение");
                final long chatId=id;
                final String peerName=name;
                LinearLayout card=new LinearLayout(this);
                card.setOrientation(1);
                card.setPadding(dp(14),dp(12),dp(14),dp(12));
                card.setBackground(round(PANEL,16));
                card.addView(label(peerName,16,TEXT,true));
                card.addView(label(last.isEmpty()?"Нет сообщений":last,14,MUTED,false));
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);
                lp.bottomMargin=dp(8);
                chatsList.addView(card,lp);
                card.setOnClickListener(v->openChat(chatId,peerName));
            }
        },null,quiet);
    }

    private void searchUsers(EditText query,Button search,LinearLayout results){
        String value=query.getText().toString().trim();
        if(value.startsWith("@"))value=value.substring(1).trim();
        if(value.length()<2){
            toast("Введите минимум 2 символа");
            return;
        }
        final String searchValue=value;
        search.setEnabled(false);
        requestGet("/users/search?q="+enc(searchValue),r->{
            search.setEnabled(true);
            results.removeAllViews();
            JSONArray users=r.optJSONArray("users");
            if(users==null||users.length()==0){
                results.addView(label("Пользователи не найдены",14,MUTED,false));
                return;
            }
            for(int i=0;i<users.length();i++){
                JSONObject u=users.optJSONObject(i);
                if(u==null)continue;
                long uid=u.optLong("id");
                String username=u.optString("username");
                String displayName=u.optString("display_name");
                String title=displayName.isEmpty()?"@"+username:displayName;
                LinearLayout card=new LinearLayout(this);
                card.setOrientation(1);
                card.setPadding(dp(13),dp(10),dp(13),dp(10));
                card.setBackground(round(PANEL,14));
                card.addView(label(title,15,TEXT,true));
                card.addView(label("@"+username,12,MUTED,false));
                Button message=button("Написать");
                card.addView(message);
                message.setOnClickListener(v->{
                    JSONObject body=new JSONObject();
                    try{body.put("user_id",uid);}catch(Exception ignored){}
                    request("/chats/direct",body,true,created->{
                        String peer=displayName.isEmpty()?"@"+username:displayName;
                        openChat(created.optLong("chat_id"),peer);
                    });
                });
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);
                lp.bottomMargin=dp(8);
                results.addView(card,lp);
            }
        });
    }

    private void showUserSearch(){
        base("Новая переписка");
        EditText q=field("Имя или @username");
        content.addView(q);
        Button search=button("Найти");
        content.addView(search);
        LinearLayout results=new LinearLayout(this);
        results.setOrientation(1);
        content.addView(results);
        search.setOnClickListener(v->searchUsers(q,search,results));
        q.setOnEditorActionListener((v,action,event)->{
            searchUsers(q,search,results);
            return true;
        });
    }
    private void openChat(long id,String peer){
        currentChat=id;base(peer);
        // Replace the generic list container with the dedicated message history.
        root.removeViewAt(1);if(navBar!=null)root.removeView(navBar);
        Button back=button("‹ Чаты");back.setTextSize(13);LinearLayout.LayoutParams bp=new LinearLayout.LayoutParams(-2,-2);top.addView(back,bp);back.setOnClickListener(v->showChats());
        ScrollView history=new ScrollView(this);LinearLayout messages=new LinearLayout(this);messages.setOrientation(1);messages.setPadding(dp(10),dp(8),dp(10),dp(8));history.addView(messages);root.addView(history,new LinearLayout.LayoutParams(-1,0,1));
        LinearLayout composerBox=new LinearLayout(this);composerBox.setOrientation(1);composerBox.setPadding(dp(8),dp(5),dp(8),dp(5));composerBox.setBackgroundColor(PANEL);
        replyPreview=new LinearLayout(this);replyPreview.setOrientation(1);replyPreview.setPadding(dp(10),dp(7),dp(10),dp(7));replyPreview.setBackground(round(0xFF202A38,12));replyPreview.setVisibility(View.GONE);replyPreview.addView(label("Ответ на сообщение",12,0xFF76A7FF,true));TextView previewText=label("",13,TEXT,false);replyPreview.addView(previewText);replyPreview.setOnClickListener(v->clearReply());composerBox.addView(replyPreview,new LinearLayout.LayoutParams(-1,-2));
        LinearLayout bar=new LinearLayout(this);bar.setGravity(Gravity.CENTER_VERTICAL);composer=field("Сообщение");composer.setLayoutParams(new LinearLayout.LayoutParams(0,-2,1));bar.addView(composer);Button send=button("➤");send.setTextSize(16);LinearLayout.LayoutParams sp=new LinearLayout.LayoutParams(dp(58),dp(48));bar.addView(send,sp);composerBox.addView(bar);root.addView(composerBox);
        send.setOnClickListener(v->sendMessage(id,messages,history));composer.setOnEditorActionListener((v,a,e)->{sendMessage(id,messages,history);return true;});openMessages=messages;openHistory=history;refreshMessages(id,messages,history);
    }
    private void refreshMessages(long id,LinearLayout messages,ScrollView history){refreshMessages(id,messages,history,false);}
    /** quiet=true is used by realtime sync: a failed background refresh must not pop up a dialog. */
    private void refreshMessages(long id,LinearLayout messages,ScrollView history,boolean quiet){
        request("/chats/"+id+"/messages",null,true,r->{
            if(currentChat!=id||openMessages!=messages)return;
            // Keep the reading position unless the user is already at the bottom.
            final boolean stick=messages.getChildCount()==0||messages.getBottom()-(history.getScrollY()+history.getHeight())<dp(80);
            final int oldY=history.getScrollY();
            messages.removeAllViews();
            JSONArray a=r.optJSONArray("messages");
            if(a!=null)for(int i=0;i<a.length();i++){
                JSONObject m=a.optJSONObject(i);
                if(m==null)continue;
                boolean mine=m.optBoolean("is_mine");
                long messageId=m.optLong("id",0);
                String body=m.optString("body","");
                LinearLayout row=new LinearLayout(this);row.setTag(Long.valueOf(messageId));row.setOrientation(1);
                row.setGravity(mine?Gravity.RIGHT:Gravity.LEFT);
                long replyId=m.optLong("reply_to_message_id",0);String quoted=m.optString("reply_to_body","");String quotedSender=m.optString("reply_to_sender_username","");
                if(replyId>0){TextView quote=label((quotedSender.isEmpty()?"Ответ на сообщение":"Ответ на @"+quotedSender)+"\\n"+(quoted.isEmpty()?"Сообщение недоступно":quoted),12,MUTED,false);quote.setPadding(dp(9),dp(6),dp(9),dp(6));quote.setBackground(round(0xFF111925,10));LinearLayout.LayoutParams qp=new LinearLayout.LayoutParams(-2,-2);qp.setMargins(dp(2),dp(4),dp(2),0);row.addView(quote,qp);quote.setOnClickListener(v->scrollToMessage(messages,history,replyId));}
                String forwardedFrom=m.optString("forwarded_from_username","");
                if(!forwardedFrom.isEmpty()){TextView fwd=label("Переслано от @"+forwardedFrom,12,MUTED,false);fwd.setPadding(dp(6),0,dp(6),0);row.addView(fwd);}
                TextView bubble=label(body,15,TEXT,false);
                bubble.setPadding(dp(13),dp(9),dp(13),dp(9));
                bubble.setBackground(round(mine?0xFF17466A:0xFF202A38,16));
                LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-2,-2);
                p.setMargins(dp(2),dp(4),dp(2),dp(4));
                row.addView(bubble,p);
                JSONArray reactions=m.optJSONArray("reactions");
                final Set<String> mineEmojis=new HashSet<>();
                if(reactions!=null)for(int k=0;k<reactions.length();k++){JSONObject o=reactions.optJSONObject(k);if(o!=null&&o.optBoolean("mine"))mineEmojis.add(o.optString("emoji"));}
                String flags=(m.optBoolean("pinned")?"\uD83D\uDCCC закреплено":"")+(m.has("edited_at")&&!m.isNull("edited_at")?(m.optBoolean("pinned")?" · ":"")+"изменено":"");
                if(!flags.isEmpty()){TextView meta=label(flags,11,MUTED,false);meta.setPadding(dp(6),0,dp(6),0);row.addView(meta);}
                View chips=reactions==null?null:reactionChips(reactions,id,messageId);
                if(chips!=null)row.addView(chips);
                bubble.setOnLongClickListener(v->{
                    MessageActionMenu.show(this,bubble,body,mine,mineEmojis,
                            ()->confirmDeleteMessage(id,messageId,messages,history),
                            ()->setReply(messageId,body,m.optString("sender_username","")),
                            (emoji,had)->toggleReaction(id,messageId,emoji,had));
                    return true;
                });
                messages.addView(row);
            }
            history.post(()->{if(stick)history.fullScroll(View.FOCUS_DOWN);else history.scrollTo(0,oldY);});
        },null,quiet);
    }

    /** One chip per reaction (emoji + count); tapping toggles the current user's own reaction. */
    private View reactionChips(JSONArray reactions,long chatId,long messageId){
        LinearLayout column=new LinearLayout(this);column.setOrientation(1);
        LinearLayout line=null;int inLine=0;
        for(int k=0;k<reactions.length();k++){
            JSONObject o=reactions.optJSONObject(k);
            if(o==null)continue;
            final String emoji=o.optString("emoji");
            final boolean had=o.optBoolean("mine");
            int count=o.optInt("count");
            if(emoji.isEmpty()||count<=0)continue;
            if(line==null||inLine==4){line=new LinearLayout(this);line.setOrientation(0);column.addView(line);inLine=0;}
            TextView chip=label(emoji+" "+count,13,TEXT,had);
            chip.setGravity(Gravity.CENTER_VERTICAL);
            chip.setMinHeight(dp(32));
            chip.setPadding(dp(9),dp(4),dp(9),dp(4));
            chip.setBackground(round(had?0xFF2D4F7A:0xFF1A2330,12));
            LinearLayout.LayoutParams cp=new LinearLayout.LayoutParams(-2,-2);cp.setMargins(dp(2),0,dp(2),dp(2));
            line.addView(chip,cp);
            chip.setOnClickListener(v->toggleReaction(chatId,messageId,emoji,had));
            inLine++;
        }
        return column.getChildCount()==0?null:column;
    }

    /** POST adds the reaction, DELETE ?emoji= removes it. The list is re-synced from the server afterwards. */
    private void toggleReaction(long chatId,long messageId,String emoji,boolean remove){
        if(messageId<=0||emoji==null||emoji.isEmpty())return;
        final String base="/chats/"+chatId+"/messages/"+messageId+"/reactions";
        final Done synced=r->{if(currentChat==chatId&&openMessages!=null&&openHistory!=null)refreshMessages(chatId,openMessages,openHistory,true);};
        if(remove){requestDelete(base+"?emoji="+enc(emoji),"Не удалось убрать реакцию",synced);return;}
        JSONObject body=new JSONObject();
        try{body.put("emoji",emoji);}catch(JSONException ignored){}
        request(base,body,true,synced,(err,code)->{
            String e=err.optString("error","");
            toast(code==409||"reaction_limit_reached".equals(e)?"На сообщении уже слишком много разных реакций":
                    code==404?"Сообщение недоступно":"Не удалось поставить реакцию");
        },true);
    }

    // ---- realtime (WebSocket /api/v1/ws) -------------------------------------------------
    private void ensureRealtime(){
        if(token.isEmpty())return;
        if(realtime==null)realtime=new RealtimeClient(API.replaceFirst("^http","ws")+"/ws",()->token,json->ui.post(()->onRealtime(json)));
        realtime.start(); // idempotent
    }
    private void stopRealtime(){
        if(realtime!=null)realtime.stop();
        if(pendingSync!=null){ui.removeCallbacks(pendingSync);pendingSync=null;}
    }
    private void onRealtime(String json){
        if(token.isEmpty())return;
        String type;long chat;
        try{JSONObject e=new JSONObject(json);type=e.optString("type");chat=e.optLong("chat_id",0);}catch(JSONException bad){return;}
        switch(type){
            case "ready": scheduleSync();break; // (re)connected: events may have been missed meanwhile
            case "message.new":case "message.edited":case "message.deleted":case "reaction.updated":
            case "message.pinned":case "message.unpinned":
                if(openMessages!=null&&chat==currentChat)scheduleSync();
                else if(openChatsList!=null)scheduleSync();
                break;
            default: break; // typing / presence / chat.read are not rendered on Android yet
        }
    }
    /** Coalesces bursts of events into one re-fetch of whatever list is on screen. */
    private void scheduleSync(){
        if(pendingSync!=null)ui.removeCallbacks(pendingSync);
        pendingSync=()->{
            pendingSync=null;
            if(token.isEmpty())return;
            if(openMessages!=null&&openHistory!=null&&currentChat!=0)refreshMessages(currentChat,openMessages,openHistory,true);
            else if(openChatsList!=null)loadChats(openChatsList,true);
        };
        ui.postDelayed(pendingSync,250);
    }
    // On resume the socket reconnects and its "ready" event triggers the re-sync of whatever is on screen.
    @Override protected void onResume(){super.onResume();ensureRealtime();}
    @Override protected void onPause(){super.onPause();stopRealtime();}

    private void setReply(long messageId,String body,String sender){
        replyToMessageId=messageId;replyToBody=body;replyToSender=sender;
        if(replyPreview!=null){replyPreview.removeAllViews();replyPreview.addView(label(sender.isEmpty()?"Ответ на сообщение":"Ответ на @"+sender,12,0xFF76A7FF,true));replyPreview.addView(label(body,13,TEXT,false));replyPreview.setVisibility(View.VISIBLE);}
    }
    private void clearReply(){replyToMessageId=0;replyToBody="";replyToSender="";if(replyPreview!=null)replyPreview.setVisibility(View.GONE);}
    private void scrollToMessage(LinearLayout messages,ScrollView history,long targetID){
        for(int i=0;i<messages.getChildCount();i++){View child=messages.getChildAt(i);Object tag=child.getTag();if(tag instanceof Long&&((Long)tag)==targetID){history.smoothScrollTo(0,child.getTop());child.setBackground(round(0xFF263B55,12));return;}}
        toast("Исходное сообщение не загружено");
    }

    private void confirmDeleteMessage(long chatId,long messageId,LinearLayout messages,ScrollView history){
        if(messageId<=0)return;
        KentDialog.showMessage(this,"Удалить сообщение?",
                "Оно будет удалено у всех участников переписки.","Удалить",()->{
            requestDelete("/chats/"+chatId+"/messages/"+messageId,result->{
                if(currentChat==chatId)refreshMessages(chatId,messages,history);
                toast("Сообщение удалено");
            });
        });
    }
    private void sendMessage(long id,LinearLayout messages,ScrollView history){
        if(composer==null)return;
        String text=composer.getText().toString().trim();
        if(text.isEmpty())return;
        ArrayDeque<String> queue=sendQueues.computeIfAbsent(id,k->new ArrayDeque<>());
        ArrayDeque<Long> replyQueue=replyQueues.computeIfAbsent(id,k->new ArrayDeque<>());
        // A failed head remains queued. Tapping send with that same restored draft retries it
        // instead of creating a duplicate message.
        if(queue.isEmpty()||!queue.peekFirst().equals(text)||sendingChats.contains(id)){queue.addLast(text);replyQueue.addLast(replyToMessageId);clearReply();}
        composer.setText("");
        pumpSendQueue(id,messages,history);
    }
    private void pumpSendQueue(long id,LinearLayout messages,ScrollView history){
        if(sendingChats.contains(id))return;
        ArrayDeque<String> queue=sendQueues.get(id);
        if(queue==null||queue.isEmpty())return;
        String text=queue.peekFirst();
        ArrayDeque<Long> replyQueue=replyQueues.get(id);long replyID=replyQueue==null||replyQueue.isEmpty()?0:replyQueue.peekFirst();
        sendingChats.add(id);
        JSONObject body=new JSONObject();
        try{body.put("text",text);if(replyID>0)body.put("reply_to_message_id",replyID);}catch(Exception ignored){}
        request("/chats/"+id+"/messages",body,true,
            response->{
                ArrayDeque<String> pending=sendQueues.get(id);
                if(pending!=null&&!pending.isEmpty())pending.removeFirst();
                ArrayDeque<Long> pendingReplies=replyQueues.get(id);if(pendingReplies!=null&&!pendingReplies.isEmpty())pendingReplies.removeFirst();
                sendingChats.remove(id);
                if(pending!=null&&pending.isEmpty()){sendQueues.remove(id);replyQueues.remove(id);}
                if(pending!=null&&!pending.isEmpty())pumpSendQueue(id,messages,history);
                else refreshMessages(id,messages,history);
            },
            (error,status)->{
                sendingChats.remove(id);
                if(currentChat==id&&composer!=null&&composer.getText().length()==0)composer.setText(text);
            });
    }
    private interface Done{void accept(JSONObject value);}
    private void requestGet(String path,Done done){request(path,null,true,done);}
    private interface Failed{void accept(JSONObject value,int status);}
    private void request(String path,JSONObject body,boolean auth,Done done){
        request(path,body,auth,done,null);
    }
    private void request(String path,JSONObject body,boolean auth,Done done,Failed failed){request(path,body,auth,done,failed,false);}
    private void request(String path,JSONObject body,boolean auth,Done done,Failed failed,boolean quiet){
        io.execute(()->{JSONObject out=new JSONObject();int status=0;try{HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();c.setRequestMethod(body==null?"GET":"POST");c.setConnectTimeout(12000);c.setReadTimeout(12000);c.setRequestProperty("Accept","application/json");if(auth&&!token.isEmpty())c.setRequestProperty("Authorization","Bearer "+token);if(body!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");try(OutputStream s=c.getOutputStream()){s.write(body.toString().getBytes(StandardCharsets.UTF_8));}}status=c.getResponseCode();InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();if(in!=null){ByteArrayOutputStream b=new ByteArrayOutputStream();byte[] buf=new byte[4096];int n;while((n=in.read(buf))>0)b.write(buf,0,n);String responseText=b.toString("UTF-8");try{out=new JSONObject(responseText);}catch(JSONException parseError){out=new JSONObject();out.put("error",status==404?"endpoint_not_found":status>=500?"server_error":"http_error");}}c.disconnect();}catch(Exception e){try{out.put("error","connection_failed");}catch(Exception ignored){}}final JSONObject result=out;final int code=status;runOnUiThread(()->{
            if(code>=200&&code<300){done.accept(result);return;}
            if(code==401&&auth){prefs.edit().clear().apply();token="";showAuth();toast("Войдите снова");}
            else if(!quiet)KentDialog.showMessage(this,code==404?"Маршрут не найден":"Не удалось выполнить запрос",result.optString("error","Ошибка соединения")+" ("+code+")","Понятно",null);
            if(failed!=null)failed.accept(result,code);
        });});
    }
    private void requestDelete(String path,Done done){requestDelete(path,"Не удалось удалить сообщение",done);}
    private void requestDelete(String path,String failTitle,Done done){
        io.execute(()->{
            JSONObject out=new JSONObject();
            int status=0;
            try{
                HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();
                c.setRequestMethod("DELETE");
                c.setConnectTimeout(12000);
                c.setReadTimeout(12000);
                c.setRequestProperty("Accept","application/json");
                if(!token.isEmpty())c.setRequestProperty("Authorization","Bearer "+token);
                status=c.getResponseCode();
                InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();
                if(in!=null){
                    ByteArrayOutputStream b=new ByteArrayOutputStream();
                    byte[] buf=new byte[4096];
                    int n;
                    while((n=in.read(buf))>0)b.write(buf,0,n);
                    try{out=new JSONObject(b.toString("UTF-8"));}catch(JSONException ignored){}
                }
                c.disconnect();
            }catch(Exception e){
                try{out.put("error","connection_failed");}catch(Exception ignored){}
            }
            final JSONObject result=out;
            final int code=status;
            runOnUiThread(()->{
                if(code>=200&&code<300)done.accept(result);
                else if(code==401){
                    prefs.edit().clear().apply();
                    token="";
                    showAuth();
                }else{
                    KentDialog.showMessage(this,failTitle,
                            result.optString("error","Ошибка соединения")+" ("+code+")","Понятно",null);
                }
            });
        });
    }
    private String enc(String s){try{return URLEncoder.encode(s,"UTF-8");}catch(Exception e){return s;}}
    private EditText field(String hint){EditText e=new EditText(this);e.setTextColor(TEXT);e.setHintTextColor(MUTED);e.setHint(hint);e.setPadding(dp(13),dp(8),dp(13),dp(8));e.setBackground(round(PANEL,12));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(9);e.setLayoutParams(p);return e;}
    private Button button(String s){Button b=new Button(this);b.setText(s);b.setTextColor(TEXT);b.setTextSize(14);b.setTypeface(null,Typeface.BOLD);b.setAllCaps(false);b.setMinHeight(dp(48));b.setPadding(dp(18),dp(8),dp(18),dp(8));b.setBackground(round(0xFF17649A,28));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(8);b.setLayoutParams(p);return b;}
    private TextView label(String s,int size,int color,boolean bold){TextView t=new TextView(this);t.setText(s);t.setTextSize(size);t.setTextColor(color);if(bold)t.setTypeface(null,Typeface.BOLD);return t;}
    private android.graphics.drawable.GradientDrawable round(int color,int radius){android.graphics.drawable.GradientDrawable d=new android.graphics.drawable.GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));return d;}
    private int dp(float n){return (int)(n*getResources().getDisplayMetrics().density+0.5f);}
    private void toast(String s){KentDialog.showToast(this,s);}
    @Override protected void onDestroy(){super.onDestroy();stopRealtime();io.shutdown();}
}