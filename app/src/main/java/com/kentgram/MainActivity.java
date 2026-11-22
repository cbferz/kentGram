package com.kentgram;

import android.app.Activity;
import android.app.AlertDialog;
import android.animation.ValueAnimator;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
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
import android.net.Uri;
import android.database.Cursor;
import android.provider.OpenableColumns;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.UUID;

public final class MainActivity extends Activity {
    private static final String API="https://api.kentgram.tendercheck.pro/api/v1";
    private static final int BG=0xFF090B10,PANEL=0xFF121720,TEXT=0xFFF4F6FA,MUTED=0xFF8994A6,ACCENT=0xFF648DFF,PANEL_ALT=0xFF1A2230;
    private final ExecutorService io=Executors.newSingleThreadExecutor();
    private OkHttpClient realtimeHttp;
    private volatile WebSocket realtimeSocket;
    private volatile boolean realtimeConnecting=false, realtimeReconnectScheduled=false;
    private Runnable realtimeReconnectTask;
    private LinearLayout activeMessages, activeChatsList;
    private ScrollView activeHistory;
    private TextView activeChatStatus;
    private String activeChatSubtitle="";
    private long lastTypingSentAt=0;
    private SharedPreferences prefs;
    private String token="";
    private LinearLayout root,content,top,navBar;
    private EditText composer;
    private LinearLayout replyPreview;
    private long replyToMessageId=0;
    private String replyToBody="", replyToSender="";
    private long currentChat=0, activePeerID=0, activePeerReadCursor=0;
    private static final int REQUEST_PICK_CHAT_MEDIA=4101;
    private long pendingMediaChatId=0;
    private LinearLayout pendingMediaMessages;
    private ScrollView pendingMediaHistory;
    private final HashMap<Long,ArrayDeque<String>> sendQueues=new HashMap<>();
    private final HashMap<Long,ArrayDeque<Long>> replyQueues=new HashMap<>();
    private final HashSet<Long> sendingChats=new HashSet<>();
    private final Handler uiHandler=new Handler(Looper.getMainLooper());
    private View highlightedMessage;
    private Runnable clearHighlightTask;
    private String activePeerUsername="", activePeerAvatarUrl="";
    private JSONArray cachedChats;
    private final HashMap<String,Bitmap> avatarCache=new HashMap<>();
    private final HashMap<String,Bitmap> mediaCache=new HashMap<>();
    private final HashMap<String,ArrayList<ImageView>> mediaTargets=new HashMap<>();
    private final HashSet<String> mediaLoading=new HashSet<>();
    private final HashSet<String> avatarLoading=new HashSet<>();
    private final HashMap<String,ArrayList<ImageView>> avatarTargets=new HashMap<>();
    private boolean onChatScreen=false, onNewChatScreen=false;

    @Override public void onCreate(Bundle state){super.onCreate(state);getWindow().setStatusBarColor(Color.BLACK);getWindow().setNavigationBarColor(BG);getWindow().setBackgroundDrawable(new android.graphics.drawable.ColorDrawable(BG));getWindow().setSoftInputMode(android.view.WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE|android.view.WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);prefs=getSharedPreferences("kentgram",MODE_PRIVATE);token=prefs.getString("access_token","");if(token.isEmpty())showAuth();else showChats();}

    private void base(String title){
        activeChatsList=null;
        activeMessages=null;
        activeHistory=null;
        activeChatStatus=null;
        root=new LinearLayout(this);
        root.setOrientation(1);
        root.setFocusableInTouchMode(true);
        root.setBackgroundColor(BG);

        top=new LinearLayout(this);
        top.setGravity(Gravity.CENTER_VERTICAL);
        top.setPadding(dp(20),dp(16),dp(20),dp(12));
        top.setBackgroundColor(Color.TRANSPARENT);
        TextView h=label(title,23,TEXT,true);
        h.setLetterSpacing(-0.025f);
        top.addView(h,new LinearLayout.LayoutParams(0,-2,1));
        LinearLayout.LayoutParams headerParams=new LinearLayout.LayoutParams(-1,-2);
        headerParams.setMargins(dp(2),dp(4),dp(2),dp(2));
        root.addView(top,headerParams);

        ScrollView scroll=new ScrollView(this);
        scroll.setClipToPadding(false);
        scroll.setFillViewport(true);
        content=new LinearLayout(this);
        content.setOrientation(1);
        content.setPadding(dp(18),dp(12),dp(18),dp(22));
        scroll.addView(content);
        root.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));

        buildNavigation(title);
        setContentView(root);
        content.setAlpha(0f);
        content.setTranslationY(dp(9));
        content.animate().alpha(1f).translationY(0f).setDuration(210)
                .setInterpolator(new android.view.animation.DecelerateInterpolator()).start();
        if(navBar!=null){navBar.setAlpha(0f);navBar.animate().alpha(1f).setDuration(190).start();}
    }

    private void buildNavigation(String title){
        navBar=new LinearLayout(this);
        navBar.setGravity(Gravity.CENTER);
        navBar.setOrientation(LinearLayout.HORIZONTAL);
        navBar.setPadding(dp(7),dp(7),dp(7),dp(7));
        navBar.setBackground(round(0xFF151B25,30));
        navBar.setElevation(dp(8));

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
            item.setPadding(dp(2),dp(7),dp(2),dp(7));
            item.setBackground(round(selected?0xFF253653:Color.TRANSPARENT,22));

            TextView icon=label(icons[i],20,selected?ACCENT:0xFF9AA5B5,true);
            icon.setGravity(Gravity.CENTER);
            TextView text=label(labels[i],10,selected?0xFFE8EEFF:0xFF9AA5B5,true);
            text.setGravity(Gravity.CENTER);
            text.setMaxLines(1);
            text.setIncludeFontPadding(false);
            item.addView(icon);
            item.addView(text);

            LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(0,dp(58),1);
            lp.setMargins(dp(2),0,dp(2),0);
            navBar.addView(item,lp);
            item.setContentDescription(labels[i]);
            item.setOnClickListener(v->navigate(screen));
            item.setOnTouchListener((v,event)->{
                if(event.getAction()==android.view.MotionEvent.ACTION_DOWN)
                    v.animate().scaleX(.96f).scaleY(.96f).setDuration(80).start();
                else if(event.getAction()==android.view.MotionEvent.ACTION_UP ||
                        event.getAction()==android.view.MotionEvent.ACTION_CANCEL)
                    v.animate().scaleX(1f).scaleY(1f).setDuration(110).start();
                return false;
            });
        }
        LinearLayout.LayoutParams navParams=new LinearLayout.LayoutParams(-1,-2);
        navParams.setMargins(dp(14),dp(5),dp(14),dp(10));
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
        TextView heading=label("Люди",18,TEXT,true);
        content.addView(heading);
        TextView sub=label("Находи знакомых и начинай общение.",13,MUTED,false);
        LinearLayout.LayoutParams subp=new LinearLayout.LayoutParams(-1,-2);
        subp.topMargin=dp(4);
        subp.bottomMargin=dp(14);
        content.addView(sub,subp);

        LinearLayout searchBox=new LinearLayout(this);
        searchBox.setGravity(Gravity.CENTER_VERTICAL);
        searchBox.setPadding(dp(12),0,dp(8),0);
        searchBox.setBackground(round(PANEL,17));
        EditText query=field("Имя или @username");
        query.setSingleLine(true);
        query.setBackgroundColor(Color.TRANSPARENT);
        query.setLayoutParams(new LinearLayout.LayoutParams(0,dp(50),1));
        searchBox.addView(query);
        TextView search=label("⌕",25,ACCENT,true);
        search.setGravity(Gravity.CENTER);
        searchBox.addView(search,new LinearLayout.LayoutParams(dp(42),dp(46)));
        content.addView(searchBox);

        LinearLayout results=new LinearLayout(this);
        results.setOrientation(1);
        LinearLayout.LayoutParams rp=new LinearLayout.LayoutParams(-1,-2);
        rp.topMargin=dp(8);
        content.addView(results,rp);

        TextView contactsTitle=label("Мои контакты",15,TEXT,true);
        contactsTitle.setPadding(0,dp(18),0,dp(9));
        content.addView(contactsTitle);
        LinearLayout contactsList=new LinearLayout(this);
        contactsList.setOrientation(1);
        content.addView(contactsList);
        requestGet("/contacts",r->{
            contactsList.removeAllViews();
            JSONArray contacts=r.optJSONArray("contacts");
            if(contacts==null||contacts.length()==0){
                TextView empty=label("Контактов пока нет. Найди человека через поиск выше и открой его профиль.",13,MUTED,false);
                empty.setPadding(dp(14),dp(16),dp(14),dp(16));
                empty.setBackground(round(PANEL,18));
                contactsList.addView(empty);
            } else for(int i=0;i<contacts.length();i++){
                JSONObject u=contacts.optJSONObject(i);
                if(u==null)continue;
                String username=u.optString("username"),name=u.optString("display_name");
                String display=name.isEmpty()?"@"+username:name;
                LinearLayout card=new LinearLayout(this);
                card.setGravity(Gravity.CENTER_VERTICAL);
                card.setPadding(dp(12),dp(10),dp(12),dp(10));
                card.setBackground(round(PANEL,18));
                TextView avatar=label(display.substring(0,1).toUpperCase(),17,0xFFEAF0FF,true);
                avatar.setGravity(Gravity.CENTER);
                avatar.setBackground(round(0xFF283957,50));
                card.addView(avatar,new LinearLayout.LayoutParams(dp(44),dp(44)));
                LinearLayout info=new LinearLayout(this);
                info.setOrientation(1);
                info.setPadding(dp(11),0,0,0);
                info.addView(label(display,14,TEXT,true));
                info.addView(label("@"+username,12,MUTED,false));
                card.addView(info,new LinearLayout.LayoutParams(0,-2,1));
                TextView open=label("›",25,ACCENT,true);
                open.setGravity(Gravity.CENTER);
                card.addView(open,new LinearLayout.LayoutParams(dp(30),dp(42)));
                open.setOnClickListener(v->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);
                lp.bottomMargin=dp(7);
                contactsList.addView(card,lp);
                card.setOnClickListener(v->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
            }
        });

        Runnable doSearch=()->{
            String q=query.getText().toString().trim();
            if(q.length()<2){toast("Введите минимум 2 символа");return;}
            requestGet("/users/search?q="+enc(q),r->{
                results.removeAllViews();
                JSONArray users=r.optJSONArray("users");
                if(users==null||users.length()==0){
                    TextView empty=label("Ничего не найдено",13,MUTED,false);
                    empty.setPadding(dp(12),dp(12),dp(12),dp(12));
                    results.addView(empty);
                    return;
                }
                for(int i=0;i<users.length();i++){
                    JSONObject u=users.optJSONObject(i);
                    if(u==null)continue;
                    String username=u.optString("username"),name=u.optString("display_name");
                    String display=name.isEmpty()?"@"+username:name;
                    LinearLayout card=new LinearLayout(this);
                    card.setGravity(Gravity.CENTER_VERTICAL);
                    card.setPadding(dp(12),dp(10),dp(12),dp(10));
                    card.setBackground(round(PANEL_ALT,17));
                    TextView avatar=label(display.substring(0,1).toUpperCase(),16,0xFFEAF0FF,true);
                    avatar.setGravity(Gravity.CENTER);
                    avatar.setBackground(round(0xFF283957,50));
                    card.addView(avatar,new LinearLayout.LayoutParams(dp(42),dp(42)));
                    LinearLayout info=new LinearLayout(this);
                    info.setOrientation(1);
                    info.setPadding(dp(10),0,0,0);
                    info.addView(label(display,14,TEXT,true));
                    info.addView(label("@"+username,12,MUTED,false));
                    card.addView(info,new LinearLayout.LayoutParams(0,-2,1));
                    TextView open=label("Профиль  ›",12,ACCENT,true);
                    card.addView(open);
                    open.setOnClickListener(v->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
                    LinearLayout.LayoutParams cp=new LinearLayout.LayoutParams(-1,-2);
                    cp.bottomMargin=dp(7);
                    results.addView(card,cp);
                    card.setOnClickListener(v->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
                }
            });
        };
        search.setOnClickListener(v->doSearch.run());
        query.setOnEditorActionListener((v,action,event)->{doSearch.run();return true;});
    }

    private void showFeed(){
        base("Лента");
        TextView heading=label("Твоя лента",19,TEXT,true);
        content.addView(heading);
        LinearLayout empty=new LinearLayout(this);
        empty.setOrientation(1);
        empty.setGravity(Gravity.CENTER);
        empty.setPadding(dp(22),dp(38),dp(22),dp(38));
        empty.setBackground(round(PANEL,22));
        TextView mark=label("✦",34,ACCENT,true);
        mark.setGravity(Gravity.CENTER);
        empty.addView(mark);
        TextView title=label("Пока ничего нового",16,TEXT,true);
        title.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams tp=new LinearLayout.LayoutParams(-1,-2);
        tp.topMargin=dp(12);
        empty.addView(title,tp);
        TextView hint=label("Когда появятся публикации от людей, на которых ты подписан, они будут здесь.",13,MUTED,false);
        hint.setGravity(Gravity.CENTER);
        hint.setLineSpacing(dp(3),1f);
        LinearLayout.LayoutParams hp=new LinearLayout.LayoutParams(-1,-2);
        hp.topMargin=dp(6);
        empty.addView(hint,hp);
        LinearLayout.LayoutParams ep=new LinearLayout.LayoutParams(-1,-2);
        ep.topMargin=dp(16);
        content.addView(empty,ep);
    }

    private void showSettings(){
        base("Настройки");
        TextView heading=label("Аккаунт",11,MUTED,true);
        heading.setLetterSpacing(.12f);
        content.addView(heading);

        LinearLayout profileCard=new LinearLayout(this);
        profileCard.setGravity(Gravity.CENTER_VERTICAL);
        profileCard.setPadding(dp(14),dp(13),dp(14),dp(13));
        profileCard.setBackground(round(PANEL,19));
        TextView avatar=label("●",20,ACCENT,true);
        avatar.setGravity(Gravity.CENTER);
        avatar.setBackground(round(0xFF263957,50));
        profileCard.addView(avatar,new LinearLayout.LayoutParams(dp(46),dp(46)));
        LinearLayout profileInfo=new LinearLayout(this);
        profileInfo.setOrientation(1);
        profileInfo.setPadding(dp(12),0,0,0);
        profileInfo.addView(label("Мой профиль",15,TEXT,true));
        profileInfo.addView(label("Имя, описание и аватар",12,MUTED,false));
        profileCard.addView(profileInfo,new LinearLayout.LayoutParams(0,-2,1));
        profileCard.addView(label("›",25,ACCENT,true));
        content.addView(profileCard);
        profileCard.setOnClickListener(v->startActivity(new Intent(this,ProfileActivity.class)));

        TextView general=label("ПРИЛОЖЕНИЕ",10,MUTED,true);
        general.setLetterSpacing(.12f);
        general.setPadding(dp(2),dp(22),0,dp(9));
        content.addView(general);
        LinearLayout logoutCard=new LinearLayout(this);
        logoutCard.setGravity(Gravity.CENTER_VERTICAL);
        logoutCard.setPadding(dp(14),dp(14),dp(14),dp(14));
        logoutCard.setBackground(round(PANEL,18));
        TextView logoutTitle=label("Выйти из аккаунта",14,0xFFFF8995,true);
        logoutCard.addView(logoutTitle,new LinearLayout.LayoutParams(0,-2,1));
        logoutCard.addView(label("↗",18,0xFFFF8995,true));
        content.addView(logoutCard);
        logoutCard.setOnClickListener(v->KentDialog.showMessage(this,"Выйти из аккаунта?","Тебе потребуется снова войти, чтобы пользоваться kentGram.","Выйти",()->{
            request("/auth/logout",new JSONObject(),true,r->{});
            prefs.edit().clear().apply();
            token="";
            stopRealtime();
            showAuth();
        }));
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
        TextView mark=label("k",38,0xFF9AB5FF,true);
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

        EditText username=field("Имя пользователя");
        username.setSingleLine(true);
        EditText email=field("Электронная почта");
        email.setSingleLine(true);
        email.setInputType( android.text.InputType.TYPE_CLASS_TEXT |
                android.text.InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS);
        email.setVisibility(View.GONE);
        EditText password=field("Пароль");
        password.setSingleLine(true);
        password.setInputType(129);
        form.addView(username);
        form.addView(email);
        form.addView(password);

        Button primary=button("Войти");
        Button secondary=button("Создать аккаунт");
        primary.setTextSize(16);
        primary.setMinHeight(dp(60));
        primary.setBackground(round(ACCENT,40));
        secondary.setTextSize(14);
        secondary.setMinHeight(dp(48));
        secondary.setBackground(round(PANEL_ALT,40));
        form.addView(primary);
        LinearLayout.LayoutParams secondaryParams=new LinearLayout.LayoutParams(-1,-2);
        secondaryParams.topMargin=dp(4);
        secondary.setLayoutParams(secondaryParams);
        form.addView(secondary);

        final boolean[] registering={false};
        primary.setOnClickListener(v->{
            String name=username.getText().toString().trim();
            String mail=email.getText().toString().trim();
            String pass=password.getText().toString();
            if(name.isEmpty()||pass.isEmpty()||(registering[0]&&mail.isEmpty())){
                KentDialog.showMessage(this,"Заполни поля",
                        registering[0]?"Укажи имя пользователя, электронную почту и пароль.":
                                "Укажи имя пользователя и пароль.",
                        "Понятно",null);
                return;
            }
            if(registering[0]&&!android.util.Patterns.EMAIL_ADDRESS.matcher(mail).matches()){
                KentDialog.showMessage(this,"Проверь почту","Введи корректный адрес электронной почты.","Понятно",null);
                return;
            }
            auth(name,registering[0]?mail:null,pass);
        });
        secondary.setOnClickListener(v->{
            registering[0]=!registering[0];
            email.setVisibility(registering[0]?View.VISIBLE:View.GONE);
            formTitle.setText(registering[0]?"Создание аккаунта":"Вход в аккаунт");
            primary.setText(registering[0]?"Зарегистрироваться":"Войти");
            secondary.setText(registering[0]?"Уже есть аккаунт? Войти":"Создать аккаунт");
        });
    }

    private void auth(String username,String email,String password){
        JSONObject b=new JSONObject();
        try{
            b.put("username",username.trim());
            b.put("password",password);
            b.put("device_name","kentGram Android");
            if(email!=null)b.put("email",email.trim());
        }catch(Exception ignored){}
        request(email==null?"/auth/login":"/auth/register",b,false,r->{
            token=r.optString("access_token","");
            if(token.isEmpty()){toast("Сервер не вернул токен");return;}
            prefs.edit().putString("access_token",token)
                    .putString("refresh_token",r.optString("refresh_token","")).apply();
            showChats();
        });
    }
    private void showChats(){
        ensureRealtimeConnected();
        onChatScreen=false;onNewChatScreen=false;
        activePeerUsername="";activePeerAvatarUrl="";
        currentChat=0;
        base("kentGram");

        TextView heading=label("Сообщения",18,TEXT,true);
        heading.setPadding(0,dp(2),0,dp(12));
        content.addView(heading);

        LinearLayout searchRow=new LinearLayout(this);
        searchRow.setGravity(Gravity.CENTER_VERTICAL);
        searchRow.setPadding(dp(12),0,dp(6),0);
        searchRow.setBackground(round(PANEL,18));
        EditText query=field("Найти людей по имени или @username");
        query.setBackgroundColor(Color.TRANSPARENT);
        query.setSingleLine(true);
        query.setLayoutParams(new LinearLayout.LayoutParams(0,dp(50),1));
        searchRow.addView(query);
        TextView searchIcon=label("⌕",25,ACCENT,true);
        searchIcon.setGravity(Gravity.CENTER);
        searchIcon.setContentDescription("Найти");
        searchRow.addView(searchIcon,new LinearLayout.LayoutParams(dp(42),dp(46)));
        content.addView(searchRow);
        LinearLayout.LayoutParams srp=(LinearLayout.LayoutParams)searchRow.getLayoutParams();
        if(srp!=null)srp.bottomMargin=dp(18);

        LinearLayout results=new LinearLayout(this);
        results.setOrientation(1);
        content.addView(results);

        LinearLayout section=new LinearLayout(this);
        section.setGravity(Gravity.CENTER_VERTICAL);
        TextView caption=label("Последние",15,TEXT,true);
        section.addView(caption,new LinearLayout.LayoutParams(0,-2,1));
        TextView joinCommunity=label("Вступить",11,MUTED,true);
        joinCommunity.setPadding(dp(8),dp(7),dp(8),dp(7));
        joinCommunity.setBackground(round(PANEL_ALT,12));
        joinCommunity.setOnClickListener(v->showJoinCommunityDialog());
        section.addView(joinCommunity);
        TextView createCommunity=label("+ Создать",11,ACCENT,true);
        createCommunity.setPadding(dp(8),dp(7),dp(8),dp(7));
        createCommunity.setBackground(round(PANEL_ALT,12));
        createCommunity.setOnClickListener(v->showCommunityCreateDialog());
        LinearLayout.LayoutParams createParams=new LinearLayout.LayoutParams(-2,-2);
        createParams.leftMargin=dp(7);
        section.addView(createCommunity,createParams);
        content.addView(section);
        LinearLayout chatsList=new LinearLayout(this);
        chatsList.setOrientation(1);
        LinearLayout.LayoutParams clp=new LinearLayout.LayoutParams(-1,-2);
        clp.topMargin=dp(10);
        content.addView(chatsList,clp);
        activeChatsList=chatsList;
        loadChats(chatsList);

        searchRow.setOnClickListener(v->{query.requestFocus();});
        searchIcon.setOnClickListener(v->searchUsers(query,searchIcon,results));
        query.setOnEditorActionListener((v,action,event)->{
            searchUsers(query,new Button(this),results);
            return true;
        });

        LinearLayout fabBar=new LinearLayout(this);
        fabBar.setGravity(Gravity.RIGHT|Gravity.CENTER_VERTICAL);
        fabBar.setPadding(dp(16),dp(2),dp(18),dp(10));
        TextView fab=label("+",29,Color.WHITE,false);
        fab.setGravity(Gravity.CENTER);
        fab.setBackground(round(ACCENT,50));
        fab.setElevation(dp(9));
        fab.setContentDescription("Новая переписка");
        fab.setOnClickListener(v->showUserSearch());
        fab.setOnTouchListener((v,event)->{
            if(event.getAction()==android.view.MotionEvent.ACTION_DOWN)
                v.animate().scaleX(.92f).scaleY(.92f).setDuration(75).start();
            else if(event.getAction()==android.view.MotionEvent.ACTION_UP ||
                    event.getAction()==android.view.MotionEvent.ACTION_CANCEL)
                v.animate().scaleX(1f).scaleY(1f).setDuration(110).start();
            return false;
        });
        fabBar.addView(fab,new LinearLayout.LayoutParams(dp(58),dp(58)));
        int navIndex=root.indexOfChild(navBar);
        root.addView(fabBar,navIndex<0?root.getChildCount():navIndex,
                new LinearLayout.LayoutParams(-1,-2));
    }

    private void showCommunityCreateDialog(){
        LinearLayout form=new LinearLayout(this);
        form.setOrientation(1);
        form.setPadding(dp(18),dp(8),dp(18),0);
        EditText titleInput=field("Название");
        titleInput.setSingleLine(true);
        form.addView(titleInput,new LinearLayout.LayoutParams(-1,dp(52)));
        RadioGroup typeGroup=new RadioGroup(this);
        typeGroup.setOrientation(RadioGroup.HORIZONTAL);
        RadioButton groupOption=new RadioButton(this);
        groupOption.setText("Группа");
        groupOption.setTextColor(TEXT);
        groupOption.setId(View.generateViewId());
        RadioButton channelOption=new RadioButton(this);
        channelOption.setText("Канал");
        channelOption.setTextColor(TEXT);
        channelOption.setId(View.generateViewId());
        typeGroup.addView(groupOption);
        typeGroup.addView(channelOption);
        groupOption.setChecked(true);
        form.addView(typeGroup);
        EditText descriptionInput=field("Описание (необязательно)");
        descriptionInput.setSingleLine(false);
        descriptionInput.setMinLines(2);
        form.addView(descriptionInput,new LinearLayout.LayoutParams(-1,dp(76)));
        CheckBox publicToggle=new CheckBox(this);
        publicToggle.setText("Публичное сообщество");
        publicToggle.setTextColor(TEXT);
        form.addView(publicToggle);
        EditText usernameInput=field("Публичный @username");
        usernameInput.setSingleLine(true);
        usernameInput.setVisibility(View.GONE);
        form.addView(usernameInput,new LinearLayout.LayoutParams(-1,dp(50)));
        publicToggle.setOnCheckedChangeListener((button,checked)->usernameInput.setVisibility(checked?View.VISIBLE:View.GONE));
        AlertDialog dialog=new AlertDialog.Builder(this)
                .setTitle("Новое сообщество")
                .setView(form)
                .setNegativeButton("Отмена",null)
                .setPositiveButton("Создать",null)
                .create();
        dialog.setOnShowListener(v->dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(button->{
            String title=titleInput.getText().toString().trim();
            if(title.isEmpty()||title.length()>80){toast("Название: от 1 до 80 символов");return;}
            JSONObject body=new JSONObject();
            try{
                body.put("kind",channelOption.isChecked()?"channel":"group");
                body.put("title",title);
                body.put("description",descriptionInput.getText().toString().trim());
                body.put("is_public",publicToggle.isChecked());
                if(publicToggle.isChecked())body.put("username",usernameInput.getText().toString().trim().replace("@",""));
            }catch(JSONException ignored){}
            request("/chats/community",body,true,result->{
                dialog.dismiss();
                cachedChats=null;
                showChats();
                toast(channelOption.isChecked()?"Канал создан":"Группа создана");
            });
        }));
        dialog.show();
    }

    private void showJoinCommunityDialog(){
        EditText username=field("@username сообщества");
        username.setSingleLine(true);
        new AlertDialog.Builder(this)
                .setTitle("Вступить в группу или канал")
                .setView(username)
                .setNegativeButton("Отмена",null)
                .setPositiveButton("Вступить",(dialog,which)->{
                    String handle=username.getText().toString().trim().replace("@","");
                    if(handle.isEmpty()){toast("Укажите username");return;}
                    JSONObject body=new JSONObject();
                    try{body.put("username",handle);}catch(JSONException ignored){}
                    request("/chats/join",body,true,result->{
                        cachedChats=null;
                        showChats();
                        toast("Вы вступили в сообщество");
                    });
                }).show();
    }

    private void loadChats(LinearLayout chatsList){
        if(cachedChats!=null){
            renderChats(chatsList,cachedChats);
        }else{
            TextView loading=label("Загружаем переписки…",13,MUTED,false);
            loading.setGravity(Gravity.CENTER);
            loading.setPadding(0,dp(22),0,dp(22));
            chatsList.addView(loading);
        }
        requestGet("/chats",r->{
            JSONArray fresh=r.optJSONArray("chats");
            if(fresh!=null){
                cachedChats=fresh;
                renderChats(chatsList,fresh);
            }
        });
    }

    private void renderChats(LinearLayout chatsList,JSONArray chats){
        chatsList.removeAllViews();
        if(chats==null||chats.length()==0){
            LinearLayout empty=new LinearLayout(this);
            empty.setOrientation(1);
            empty.setGravity(Gravity.CENTER);
            empty.setPadding(dp(22),dp(34),dp(22),dp(34));
            empty.setBackground(round(PANEL,22));
            TextView symbol=label("✦",30,ACCENT,true);
            symbol.setGravity(Gravity.CENTER);
            empty.addView(symbol);
            TextView title=label("Здесь пока тихо",16,TEXT,true);
            title.setGravity(Gravity.CENTER);
            LinearLayout.LayoutParams etp=new LinearLayout.LayoutParams(-1,-2);
            etp.topMargin=dp(10);
            empty.addView(title,etp);
            TextView hint=label("Начни новую переписку — она появится здесь.",13,MUTED,false);
            hint.setGravity(Gravity.CENTER);
            LinearLayout.LayoutParams ehp=new LinearLayout.LayoutParams(-1,-2);
            ehp.topMargin=dp(5);
            empty.addView(hint,ehp);
            chatsList.addView(empty);
            return;
        }
        for(int i=0;i<chats.length();i++){
            JSONObject c=chats.optJSONObject(i);
            if(c==null)continue;
            long id=c.optLong("id");
            String kind=c.optString("kind","direct");
            boolean community="group".equals(kind)||"channel".equals(kind);
            String name=community?c.optString("title",""):c.optString("peer_display_name");
            if(name.isEmpty())name=community?"Сообщество":"@"+c.optString("peer_username");
            String last=c.optString("last_message","Начните общение");
            String peerUsername=c.optString("peer_username","");
            String peerAvatarUrl=c.optString("peer_avatar_url","");
            final long chatId=id;
            final long peerId=c.optLong("peer_id",0);
            final String peerName=name;
            final String peerHandle=peerUsername;
            final String peerAvatar=peerAvatarUrl;
            final String chatKind=kind;

            LinearLayout card=new LinearLayout(this);
            card.setGravity(Gravity.CENTER_VERTICAL);
            card.setOrientation(LinearLayout.HORIZONTAL);
            card.setPadding(dp(12),dp(12),dp(12),dp(12));
            card.setBackground(round(PANEL,19));
            TextView avatar=label(name.isEmpty()?"K":name.substring(0,1).toUpperCase(),18,0xFFEAF0FF,true);
            avatar.setGravity(Gravity.CENTER);
            avatar.setBackground(round(0xFF283957,50));
            card.addView(avatar,new LinearLayout.LayoutParams(dp(50),dp(50)));

            LinearLayout details=new LinearLayout(this);
            details.setOrientation(1);
            details.setPadding(dp(12),0,0,0);
            details.addView(label(peerName,15,TEXT,true));
            TextView preview=label(last.isEmpty()?"Нет сообщений":last,13,MUTED,false);
            preview.setMaxLines(1);
            preview.setEllipsize(android.text.TextUtils.TruncateAt.END);
            LinearLayout.LayoutParams dlp=new LinearLayout.LayoutParams(0,-2,1);
            dlp.gravity=Gravity.CENTER_VERTICAL;
            card.addView(details,dlp);
            details.addView(preview);
            LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);
            lp.bottomMargin=dp(8);
            chatsList.addView(card,lp);
            card.setAlpha(0f);
            card.setTranslationY(dp(7));
            card.animate().alpha(1f).translationY(0f).setStartDelay(Math.min(i*32L,256L))
                    .setDuration(190).setInterpolator(new android.view.animation.DecelerateInterpolator()).start();
            card.setOnClickListener(v->openChat(chatId,peerName,peerHandle,peerAvatar,chatKind,peerId));
            card.setOnTouchListener((v,event)->{
                if(event.getAction()==android.view.MotionEvent.ACTION_DOWN)
                    v.animate().scaleX(.985f).scaleY(.985f).setDuration(70).start();
                else if(event.getAction()==android.view.MotionEvent.ACTION_UP ||
                        event.getAction()==android.view.MotionEvent.ACTION_CANCEL)
                    v.animate().scaleX(1f).scaleY(1f).setDuration(100).start();
                return false;
            });
        }
    }

    private void searchUsers(EditText query,View search,LinearLayout results){
        String value=query.getText().toString().trim();
        if(value.startsWith("@"))value=value.substring(1).trim();
        if(value.length()<2){
            toast("Введите минимум 2 символа");
            return;
        }
        final String searchValue=value;
        requestGet("/users/search?q="+enc(searchValue),r->{
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
                        openChat(created.optLong("chat_id"),peer,username,u.optString("avatar_url",""),"direct",uid);
                    });
                });
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);
                lp.bottomMargin=dp(8);
                results.addView(card,lp);
            }
        });
    }

    private void showUserSearch(){
        onChatScreen=false;onNewChatScreen=true;
        base("Новая переписка");
        if(navBar!=null)root.removeView(navBar);
        top.removeAllViews();
        TextView back=label("‹",32,ACCENT,false);
        back.setGravity(Gravity.CENTER);
        top.addView(back,new LinearLayout.LayoutParams(dp(42),dp(46)));
        back.setOnClickListener(v->showChats());
        TextView title=label("Новая переписка",19,TEXT,true);
        LinearLayout.LayoutParams titleParams=new LinearLayout.LayoutParams(0,-2,1);
        titleParams.leftMargin=dp(10);
        top.addView(title,titleParams);
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
    private void openChat(long id,String peer,String peerUsername,String peerAvatarUrl){
        openChat(id,peer,peerUsername,peerAvatarUrl,"direct");
    }
    private void openChat(long id,String peer,String peerUsername,String peerAvatarUrl,String chatKind){
        openChat(id,peer,peerUsername,peerAvatarUrl,chatKind,0);
    }
    private void openChat(long id,String peer,String peerUsername,String peerAvatarUrl,String chatKind,long peerID){
        getWindow().setSoftInputMode(android.view.WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE|android.view.WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);
        onChatScreen=true;onNewChatScreen=false;
        activePeerID="direct".equals(chatKind)?peerID:0;
        activePeerUsername="direct".equals(chatKind)&&peerUsername!=null?peerUsername:"";
        activePeerAvatarUrl="direct".equals(chatKind)&&peerAvatarUrl!=null?peerAvatarUrl:"";
        currentChat=id;
        base(peer);
        root.removeViewAt(1);
        if(navBar!=null)root.removeView(navBar);
        top.removeAllViews();

        TextView back=label("‹",34,ACCENT,false);
        back.setGravity(Gravity.CENTER);
        top.addView(back,new LinearLayout.LayoutParams(dp(42),dp(46)));
        back.setOnClickListener(v->showChats());

        ImageView peerAvatar=new ImageView(this);
        peerAvatar.setScaleType(ImageView.ScaleType.CENTER_CROP);
        peerAvatar.setImageDrawable(getDrawable(android.R.drawable.sym_def_app_icon));
        peerAvatar.setBackground(round(0xFF283957,50));
        peerAvatar.setClipToOutline(true);
        top.addView(peerAvatar,new LinearLayout.LayoutParams(dp(42),dp(42)));
        loadChatAvatar(peerAvatar,activePeerAvatarUrl);

        LinearLayout peerBlock=new LinearLayout(this);
        peerBlock.setOrientation(1);
        peerBlock.setPadding(dp(10),0,0,0);
        TextView peerTitle=label(peer,16,TEXT,true);
        peerBlock.addView(peerTitle);
        String chatSubtitle="direct".equals(chatKind)?
                (activePeerUsername.isEmpty()?"переписка":"@"+activePeerUsername):
                ("channel".equals(chatKind)?"канал":"группа");
        activeChatSubtitle=chatSubtitle;
        activeChatStatus=label(chatSubtitle,11,MUTED,false);
        peerBlock.addView(activeChatStatus);
        top.addView(peerBlock,new LinearLayout.LayoutParams(0,-2,1));
        View.OnClickListener openPeer=v->{
            if(!activePeerUsername.isEmpty()){
                Intent profile=new Intent(this,ProfileActivity.class);
                profile.putExtra("username",activePeerUsername);
                startActivity(profile);
            }else if(!"direct".equals(chatKind)){
                showCommunityDetails(id,chatKind);
            }
        };
        peerBlock.setOnClickListener(openPeer);
        peerAvatar.setOnClickListener(openPeer);

        ScrollView history=new ScrollView(this);
        history.setClipToPadding(false);
        LinearLayout messages=new LinearLayout(this);
        messages.setOrientation(1);
        messages.setPadding(dp(12),dp(8),dp(12),dp(14));
        history.addView(messages);
        root.addView(history,new LinearLayout.LayoutParams(-1,0,1));
        activeMessages=messages;
        activeHistory=history;

        LinearLayout composerBox=new LinearLayout(this);
        composerBox.setOrientation(1);
        composerBox.setPadding(dp(12),dp(8),dp(12),dp(10));
        composerBox.setBackgroundColor(BG);

        replyPreview=new LinearLayout(this);
        replyPreview.setOrientation(1);
        replyPreview.setPadding(dp(12),dp(9),dp(12),dp(9));
        replyPreview.setBackground(round(PANEL_ALT,14));
        replyPreview.setVisibility(View.GONE);
        replyPreview.addView(label("ОТВЕТ НА СООБЩЕНИЕ",10,ACCENT,true));
        replyPreview.addView(label("",13,TEXT,false));
        replyPreview.setOnClickListener(v->clearReply());
        LinearLayout.LayoutParams rpp=new LinearLayout.LayoutParams(-1,-2);
        rpp.bottomMargin=dp(8);
        composerBox.addView(replyPreview,rpp);

        LinearLayout bar=new LinearLayout(this);
        bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.setPadding(dp(6),dp(3),dp(6),dp(3));
        bar.setBackground(round(PANEL,28));
        composer=field("Сообщение");
        composer.setBackgroundColor(Color.TRANSPARENT);
        composer.setMinHeight(dp(46));
        composer.setPadding(dp(12),dp(7),dp(8),dp(7));
        composer.setLayoutParams(new LinearLayout.LayoutParams(0,-2,1));
        bar.addView(composer);
        TextView attach=label("＋",22,ACCENT,true);
        attach.setGravity(Gravity.CENTER);
        bar.addView(attach,new LinearLayout.LayoutParams(dp(40),dp(44)));
        attach.setOnClickListener(v->{
            pendingMediaChatId=id;
            pendingMediaMessages=messages;
            pendingMediaHistory=history;
            Intent picker=new Intent(Intent.ACTION_OPEN_DOCUMENT);
            picker.addCategory(Intent.CATEGORY_OPENABLE);
            picker.setType("*/*");
            picker.putExtra(Intent.EXTRA_MIME_TYPES,new String[]{"image/jpeg","image/png","application/pdf"});
            startActivityForResult(picker,REQUEST_PICK_CHAT_MEDIA);
        });
        TextView send=label("➤",19,Color.WHITE,true);
        send.setGravity(Gravity.CENTER);
        send.setBackground(round(ACCENT,50));
        LinearLayout.LayoutParams sp=new LinearLayout.LayoutParams(dp(44),dp(44));
        bar.addView(send,sp);
        composerBox.addView(bar);
        composerBox.setVisibility(View.VISIBLE);
        composerBox.setFocusable(false);
        root.addView(composerBox,new LinearLayout.LayoutParams(-1,-2));
        composerBox.bringToFront();

        send.setOnClickListener(v->sendMessage(id,messages,history));
        composer.setOnEditorActionListener((v,a,e)->{
            sendMessage(id,messages,history);
            return true;
        });
        composer.addTextChangedListener(new android.text.TextWatcher(){
            @Override public void beforeTextChanged(CharSequence value,int start,int count,int after){}
            @Override public void onTextChanged(CharSequence value,int start,int before,int count){
                if(value.length()>0){
                    long now=System.currentTimeMillis();
                    if(now-lastTypingSentAt>1500){
                        lastTypingSentAt=now;
                        sendRealtimeTyping(id);
                    }
                }
            }
            @Override public void afterTextChanged(android.text.Editable value){}
        });
        refreshMessages(id,messages,history);
        if(!"direct".equals(chatKind)){
            requestGet("/chats/"+id,details->{
                String role=details.optString("role","member");
                boolean canPost=!"channel".equals(chatKind)||"owner".equals(role)||"admin".equals(role);
                composer.setEnabled(canPost);
                send.setEnabled(canPost);
                composer.setHint(canPost?"Сообщение":"Публиковать могут только администраторы");
                send.setAlpha(canPost?1f:.35f);
            });
        }
    }

    private void showCommunityDetails(long chatId,String chatKind){
        requestGet("/chats/"+chatId,details->{
            String title=details.optString("title","Сообщество");
            String description=details.optString("description","");
            String role=details.optString("role","member");
            long count=details.optLong("member_count",0);
            LinearLayout panel=new LinearLayout(this);
            panel.setOrientation(1);
            panel.setPadding(dp(18),dp(8),dp(18),dp(4));
            TextView summary=label((description.isEmpty()?"":description+"\n")+"Участников: "+count+"\nВаша роль: "+role,13,MUTED,false);
            panel.addView(summary);
            Button members=new Button(this);
            members.setText("Участники");
            panel.addView(members);
            members.setOnClickListener(v->loadCommunityMembers(chatId));
            if("owner".equals(role)||"admin".equals(role)){
                Button invite=new Button(this);
                invite.setText("Пригласить пользователя");
                panel.addView(invite);
                invite.setOnClickListener(v->showInviteCommunityMember(chatId));
            }
            if("owner".equals(role)){
                Button transfer=new Button(this);
                transfer.setText("Передать владельца");
                panel.addView(transfer);
                transfer.setOnClickListener(v->showTransferCommunityOwnership(chatId));
            }
            Button leave=new Button(this);
            leave.setText("Покинуть сообщество");
            panel.addView(leave);
            leave.setOnClickListener(v->{
                request("/chats/"+chatId+"/leave",new JSONObject(),true,result->{
                    cachedChats=null;
                    showChats();
                    toast("Вы покинули сообщество");
                },(error,status)->toast(status==409?"Сначала передайте права владельца":"Не удалось выйти"));
            });
            new AlertDialog.Builder(this).setTitle(title).setView(panel).setPositiveButton("Закрыть",null).show();
        });
    }

    private void loadCommunityMembers(long chatId){
        requestGet("/chats/"+chatId,details->{
            String viewerRole=details.optString("role","member");
            boolean canManage="owner".equals(viewerRole)||"admin".equals(viewerRole);
            boolean isOwner="owner".equals(viewerRole);
            requestGet("/chats/"+chatId+"/members",result->{
                JSONArray members=result.optJSONArray("members");
                if(members==null||members.length()==0){toast("Участников пока нет");return;}
                String[] labels=new String[members.length()];
                for(int i=0;i<members.length();i++){
                    JSONObject member=members.optJSONObject(i);
                    if(member==null){labels[i]="Участник";continue;}
                    String display=member.optString("display_name","");
                    String username=member.optString("username","");
                    String name=display.isEmpty()?"@"+username:display+" (@"+username+")";
                    labels[i]=name+" · "+member.optString("role","member");
                }
                new AlertDialog.Builder(this).setTitle("Участники").setItems(labels,(list,index)->{
                    if(!canManage)return;
                    JSONObject target=members.optJSONObject(index);
                    if(target==null)return;
                    String targetRole=target.optString("role","member");
                    if("owner".equals(targetRole))return;
                    long targetID=target.optLong("user_id");
                    if(targetID<=0)return;
                    String[] actions;
                    if(isOwner&&"admin".equals(targetRole))
                        actions=new String[]{"Снять администратора","Удалить администратора"};
                    else if(isOwner)
                        actions=new String[]{"Назначить администратором","Удалить участника"};
                    else if("member".equals(targetRole))
                        actions=new String[]{"Удалить участника"};
                    else return;
                    new AlertDialog.Builder(this).setTitle(labels[index]).setItems(actions,(actionDialog,action)->{
                        if(isOwner&&action==0){
                            JSONObject body=new JSONObject();
                            try{body.put("role","admin".equals(targetRole)?"member":"admin");}catch(JSONException ignored){}
                            request("/chats/"+chatId+"/members/"+targetID+"/role",body,true,updated->{
                                toast("Роль обновлена");
                                loadCommunityMembers(chatId);
                            });
                        }else{
                            requestDelete("/chats/"+chatId+"/members/"+targetID,removed->{
                                toast("Участник удалён");
                                loadCommunityMembers(chatId);
                            });
                        }
                    }).setNegativeButton("Отмена",null).show();
                }).setPositiveButton("Закрыть",null).show();
            });
        });
    }

    private void showInviteCommunityMember(long chatId){
        EditText username=field("@username пользователя");
        username.setSingleLine(true);
        new AlertDialog.Builder(this).setTitle("Пригласить участника").setView(username)
                .setNegativeButton("Отмена",null)
                .setPositiveButton("Найти",(dialog,which)->{
                    String handle=username.getText().toString().trim().replace("@","");
                    if(handle.length()<2){toast("Введите username");return;}
                    requestGet("/users/search?q="+enc(handle),result->{
                        JSONArray users=result.optJSONArray("users");
                        JSONObject found=null;
                        if(users!=null)for(int i=0;i<users.length();i++){
                            JSONObject candidate=users.optJSONObject(i);
                            if(candidate!=null&&handle.equalsIgnoreCase(candidate.optString("username"))){found=candidate;break;}
                        }
                        if(found==null){toast("Пользователь не найден");return;}
                        JSONObject body=new JSONObject();
                        try{body.put("user_id",found.optLong("id"));}catch(JSONException ignored){}
                        request("/chats/"+chatId+"/members",body,true,added->{
                            toast("Участник добавлен");
                            showCommunityDetails(chatId,"group");
                        });
                    });
                }).show();
    }

    private void showTransferCommunityOwnership(long chatId){
        EditText username=field("@username нового владельца");
        username.setSingleLine(true);
        new AlertDialog.Builder(this).setTitle("Передать владение").setView(username)
                .setNegativeButton("Отмена",null)
                .setPositiveButton("Передать",(dialog,which)->{
                    String handle=username.getText().toString().trim().replace("@","");
                    requestGet("/users/search?q="+enc(handle),result->{
                        JSONArray users=result.optJSONArray("users");
                        JSONObject found=null;
                        if(users!=null)for(int i=0;i<users.length();i++){
                            JSONObject candidate=users.optJSONObject(i);
                            if(candidate!=null&&handle.equalsIgnoreCase(candidate.optString("username"))){found=candidate;break;}
                        }
                        if(found==null){toast("Пользователь не найден");return;}
                        JSONObject body=new JSONObject();
                        try{body.put("user_id",found.optLong("id"));}catch(JSONException ignored){}
                        request("/chats/"+chatId+"/transfer-ownership",body,true,result2->{
                            toast("Владелец изменён");
                            showCommunityDetails(chatId,"group");
                        });
                    });
                }).show();
    }

    private void refreshMessages(long id,LinearLayout messages,ScrollView history){
        requestGet("/chats/"+id+"/messages",r->{
            if(currentChat!=id)return;
            JSONArray a=r.optJSONArray("messages");
            requestGet("/chats/"+id+"/read",readState->{
                if(currentChat!=id)return;
                JSONObject cursors=readState.optJSONObject("read_cursors");
                long peerRead=activePeerID>0&&cursors!=null?cursors.optLong(String.valueOf(activePeerID),0):0;
                activePeerReadCursor=peerRead;
                messages.removeAllViews();
                long latestMessageId=0;
            if(a!=null)for(int i=0;i<a.length();i++){
                JSONObject m=a.optJSONObject(i);
                if(m==null)continue;
                boolean mine=m.optBoolean("is_mine");
                long messageId=m.optLong("id",0);
                latestMessageId=Math.max(latestMessageId,messageId);
                String body=m.optString("body","");
                String mediaUrl=m.optString("media_url","");
                String mediaMime=m.optString("media_mime","");
                String mediaName=m.optString("media_name","");
                LinearLayout row=new LinearLayout(this);
                row.setTag(Long.valueOf(messageId));
                row.setOrientation(1);
                row.setGravity(mine?Gravity.RIGHT:Gravity.LEFT);

                if(!mediaUrl.isEmpty()&&mediaMime.startsWith("image/")){
                    ImageView attachment=new ImageView(this);
                    attachment.setScaleType(ImageView.ScaleType.CENTER_CROP);
                    attachment.setBackground(round(0xFF202A39,14));
                    attachment.setClipToOutline(true);
                    LinearLayout.LayoutParams attachmentParams=new LinearLayout.LayoutParams(dp(236),dp(176));
                    attachmentParams.setMargins(dp(2),dp(5),dp(2),dp(4));
                    row.addView(attachment,attachmentParams);
                    loadChatMedia(attachment,API+mediaUrl);
                    attachment.setOnClickListener(v->showChatImage(API+mediaUrl));
                }else if(!mediaUrl.isEmpty()){
                    TextView attachment=label("📄  "+(mediaName.isEmpty()?body:mediaName)+"\nФайл · "+formatBytes(m.optLong("media_size",0)),13,TEXT,true);
                    attachment.setPadding(dp(13),dp(12),dp(13),dp(12));
                    attachment.setBackground(round(PANEL_ALT,13));
                    row.addView(attachment,new LinearLayout.LayoutParams(-1,-2));
                }

                long replyId=m.optLong("reply_to_message_id",0);
                String quoted=m.optString("reply_to_body","");
                String quotedSender=m.optString("reply_to_sender_username","");
                if(replyId>0){
                    TextView quote=label((quotedSender.isEmpty()?"Ответ на сообщение":"Ответ на @"+quotedSender)+"\n"+
                            (quoted.isEmpty()?"Сообщение недоступно":quoted),11,0xFFB8C7DD,false);
                    quote.setPadding(dp(11),dp(8),dp(11),dp(8));
                    quote.setBackground(round(0xFF202A39,12));
                    LinearLayout.LayoutParams qp=new LinearLayout.LayoutParams(-2,-2);
                    qp.setMargins(dp(2),dp(5),dp(2),0);
                    row.addView(quote,qp);
                    quote.setOnClickListener(v->scrollToMessage(messages,history,replyId));
                }

                TextView bubble=label(body,15,mine?Color.WHITE:TEXT,false);
                bubble.setLineSpacing(dp(2),1.02f);
                bubble.setMaxWidth((int)(getResources().getDisplayMetrics().widthPixels*.78f));
                bubble.setPadding(dp(14),dp(10),dp(14),dp(10));
                bubble.setBackground(round(mine?0xFF315DA8:0xFF1B222D,mine?18:18));
                LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-2,-2);
                p.setMargins(dp(2),dp(4),dp(2),dp(4));
                LinearLayout messageLine=new LinearLayout(this);
                messageLine.setGravity(Gravity.CENTER_VERTICAL|(mine?Gravity.RIGHT:Gravity.LEFT));
                if(!mine){
                    ImageView senderAvatar=new ImageView(this);
                    senderAvatar.setScaleType(ImageView.ScaleType.CENTER_CROP);
                    senderAvatar.setImageDrawable(getDrawable(android.R.drawable.sym_def_app_icon));
                    senderAvatar.setBackground(round(0xFF283957,50));
                    senderAvatar.setClipToOutline(true);
                    LinearLayout.LayoutParams avatarParams=new LinearLayout.LayoutParams(dp(29),dp(29));
                    avatarParams.setMargins(0,0,dp(7),0);
                    messageLine.addView(senderAvatar,avatarParams);
                    loadChatAvatar(senderAvatar,activePeerAvatarUrl);
                }
                messageLine.addView(bubble,p);
                row.addView(messageLine,new LinearLayout.LayoutParams(-1,-2));
                if(mine&&activePeerID>0){
                    TextView receipt=label(messageId<=peerRead?"✓✓ Прочитано":"✓ Отправлено",10,
                            messageId<=peerRead?0xFF8FB4FF:MUTED,false);
                    receipt.setGravity(Gravity.RIGHT);
                    receipt.setTag(Long.valueOf(messageId));
                    row.addView(receipt,new LinearLayout.LayoutParams(-1,-2));
                }
                bubble.setOnLongClickListener(v->{
                    MessageActionMenu.show(this,bubble,body,mine,mine,
                            ()->confirmDeleteMessage(id,messageId,messages,history),
                            ()->setReply(messageId,body,m.optString("sender_username","")),
                            ()->showEditMessage(id,messageId,body,messages,history),
                            emoji->sendMessageReaction(id,messageId,emoji,messages,history));
                    return true;
                });
                messages.addView(row);
            }
            history.post(()->history.fullScroll(View.FOCUS_DOWN));
            if(latestMessageId>0){
                JSONObject readBody=new JSONObject();
                try{readBody.put("last_read_message_id",latestMessageId);}catch(JSONException ignored){}
                request("/chats/"+id+"/read",readBody,true,ignored->{});
            }
            });
        });
    }


    private void showEditMessage(long chatId,long messageId,String oldText,
                                 LinearLayout messages,ScrollView history){
        if(messageId<=0)return;

        android.app.Dialog dialog=new android.app.Dialog(this);
        LinearLayout panel=new LinearLayout(this);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setPadding(dp(20),dp(20),dp(20),dp(14));
        panel.setBackground(round(PANEL,24));

        TextView title=label("Редактировать сообщение",19,TEXT,true);
        title.setPadding(0,0,0,dp(14));
        panel.addView(title);

        EditText input=field("Введите текст сообщения");
        input.setText(oldText);
        input.setMinLines(3);
        input.setMaxLines(7);
        input.setGravity(Gravity.TOP|Gravity.LEFT);
        input.setSelection(input.length());
        panel.addView(input);

        LinearLayout buttons=new LinearLayout(this);
        buttons.setGravity(Gravity.RIGHT|Gravity.CENTER_VERTICAL);
        buttons.setOrientation(LinearLayout.HORIZONTAL);

        TextView cancel=label("Отмена",14,MUTED,true);
        cancel.setGravity(Gravity.CENTER);
        cancel.setPadding(dp(15),dp(13),dp(15),dp(13));
        buttons.addView(cancel);

        TextView save=label("Сохранить",14,Color.WHITE,true);
        save.setGravity(Gravity.CENTER);
        save.setPadding(dp(18),dp(13),dp(18),dp(13));
        save.setBackground(round(ACCENT,22));
        LinearLayout.LayoutParams saveParams=new LinearLayout.LayoutParams(-2,-2);
        saveParams.leftMargin=dp(8);
        buttons.addView(save);

        LinearLayout.LayoutParams bp=new LinearLayout.LayoutParams(-1,-2);
        bp.topMargin=dp(12);
        panel.addView(buttons,bp);

        dialog.setContentView(panel);
        android.view.Window window=dialog.getWindow();
        if(window!=null){
            window.setBackgroundDrawableResource(android.R.color.transparent);
            window.addFlags(android.view.WindowManager.LayoutParams.FLAG_DIM_BEHIND);
            android.view.WindowManager.LayoutParams attrs=window.getAttributes();
            attrs.dimAmount=0.68f;
            window.setAttributes(attrs);
        }

        cancel.setOnClickListener(v->dialog.dismiss());
        save.setOnClickListener(v->{
            String text=input.getText().toString().trim();
            if(text.isEmpty()){
                toast("Сообщение не может быть пустым");
                return;
            }
            if(text.equals(oldText)){
                dialog.dismiss();
                return;
            }
            JSONObject payload=new JSONObject();
            try{
                payload.put("text",text);
            }catch(JSONException e){
                toast("Не удалось подготовить сообщение");
                return;
            }
            save.setEnabled(false);
            requestMethod("PUT","/chats/"+chatId+"/messages/"+messageId,
                    payload,true,result->{
                        dialog.dismiss();
                        if(currentChat==chatId)refreshMessages(chatId,messages,history);
                        toast("Сообщение изменено");
                    },(error,status)->{
                        save.setEnabled(true);
                        toast(error.optString("error","Не удалось изменить сообщение"));
                    });
        });

        dialog.show();
        if(window!=null){
            window.setLayout(
                    getResources().getDisplayMetrics().widthPixels-dp(36),
                    android.view.WindowManager.LayoutParams.WRAP_CONTENT);
        }
        input.requestFocus();
        input.postDelayed(()->{
            android.view.inputmethod.InputMethodManager imm=
                    (android.view.inputmethod.InputMethodManager)
                    getSystemService(INPUT_METHOD_SERVICE);
            if(imm!=null)imm.showSoftInput(input,
                    android.view.inputmethod.InputMethodManager.SHOW_IMPLICIT);
        },180);
    }

    private void sendMessageReaction(long chatId,long messageId,String emoji,
                                     LinearLayout messages,ScrollView history){
        if(messageId<=0)return;
        JSONObject payload=new JSONObject();
        try{
            payload.put("emoji",emoji);
        }catch(JSONException e){
            toast("Не удалось добавить реакцию");
            return;
        }
        requestMethod("POST","/chats/"+chatId+"/messages/"+messageId+"/reactions",
                payload,true,result->{
                    if(currentChat==chatId)refreshMessages(chatId,messages,history);
                },(error,status)->{
                    String code=error.optString("error","");
                    if("reaction_limit_reached".equals(code)){
                        toast("Можно поставить не более 3 реакций на сообщение");
                    }else{
                        toast(code.isEmpty()?"Не удалось добавить реакцию":code);
                    }
                });
    }

    private void setReply(long messageId,String body,String sender){
        replyToMessageId=messageId;replyToBody=body;replyToSender=sender;
        if(replyPreview!=null){replyPreview.removeAllViews();replyPreview.addView(label(sender.isEmpty()?"Ответ на сообщение":"Ответ на @"+sender,12,0xFF9AB5FF,true));replyPreview.addView(label(body,13,TEXT,false));replyPreview.setVisibility(View.VISIBLE);}
    }
    private void clearReply(){replyToMessageId=0;replyToBody="";replyToSender="";if(replyPreview!=null)replyPreview.setVisibility(View.GONE);}
    private void scrollToMessage(LinearLayout messages,ScrollView history,long targetID){
        for(int i=0;i<messages.getChildCount();i++){
            View child=messages.getChildAt(i);
            Object tag=child.getTag();
            if(tag instanceof Long&&((Long)tag)==targetID){
                if(clearHighlightTask!=null)uiHandler.removeCallbacks(clearHighlightTask);
                if(highlightedMessage!=null&&highlightedMessage!=child)highlightedMessage.setBackgroundColor(Color.TRANSPARENT);
                highlightedMessage=child;
                child.setBackground(round(0xFF263B55,13));
                history.smoothScrollTo(0,Math.max(0,child.getTop()-dp(24)));
                clearHighlightTask=()->{
                    if(highlightedMessage==child){
                        ValueAnimator fade=ValueAnimator.ofObject(new android.animation.ArgbEvaluator(),0xFF263B55,0x00090B10);
                        fade.setDuration(260);
                        fade.addUpdateListener(animation->child.setBackground(round((Integer)animation.getAnimatedValue(),13)));
                        fade.addListener(new android.animation.AnimatorListenerAdapter(){
                            @Override public void onAnimationEnd(android.animation.Animator animation){
                                if(highlightedMessage==child){child.setBackgroundColor(Color.TRANSPARENT);highlightedMessage=null;}
                            }
                        });
                        fade.start();
                    }
                };
                uiHandler.postDelayed(clearHighlightTask,5000);
                return;
            }
        }
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
    private void ensureRealtimeConnected(){
        if(token.isEmpty()||realtimeSocket!=null||realtimeConnecting)return;
        realtimeConnecting=true;
        if(realtimeHttp==null){
            realtimeHttp=new OkHttpClient.Builder()
                    .readTimeout(0,TimeUnit.MILLISECONDS)
                    .pingInterval(25,TimeUnit.SECONDS)
                    .build();
        }
        Request request=new Request.Builder()
                .url(API.replace("https://","wss://")+"/realtime")
                .header("Authorization","Bearer "+token)
                .build();
        realtimeSocket=realtimeHttp.newWebSocket(request,new WebSocketListener(){
            @Override public void onOpen(WebSocket socket,Response response){
                realtimeConnecting=false;
                realtimeSocket=socket;
                realtimeReconnectScheduled=false;
            }
            @Override public void onMessage(WebSocket socket,String text){
                handleRealtimeEvent(text);
            }
            @Override public void onClosed(WebSocket socket,int code,String reason){
                if(realtimeSocket==socket)realtimeSocket=null;
                realtimeConnecting=false;
                scheduleRealtimeReconnect();
            }
            @Override public void onFailure(WebSocket socket,Throwable error,Response response){
                if(realtimeSocket==socket)realtimeSocket=null;
                realtimeConnecting=false;
                if(response!=null&&response.code()==401){
                    runOnUiThread(()->{
                        prefs.edit().clear().apply();
                        token="";
                        stopRealtime();
                        showAuth();
                        toast("Сессия завершена. Войдите снова.");
                    });
                }else{
                    scheduleRealtimeReconnect();
                }
            }
        });
    }

    private void scheduleRealtimeReconnect(){
        if(token.isEmpty()||realtimeReconnectScheduled)return;
        realtimeReconnectScheduled=true;
        realtimeReconnectTask=()->{
            realtimeReconnectScheduled=false;
            ensureRealtimeConnected();
        };
        uiHandler.postDelayed(realtimeReconnectTask,3000);
    }

    private void stopRealtime(){
        if(realtimeReconnectTask!=null)uiHandler.removeCallbacks(realtimeReconnectTask);
        realtimeReconnectTask=null;
        realtimeReconnectScheduled=false;
        realtimeConnecting=false;
        if(realtimeSocket!=null){
            realtimeSocket.close(1000,"client closing");
            realtimeSocket=null;
        }
        if(realtimeHttp!=null){
            realtimeHttp.dispatcher().executorService().shutdown();
            realtimeHttp.connectionPool().evictAll();
            realtimeHttp=null;
        }
    }

    private void sendRealtimeTyping(long chatId){
        if(realtimeSocket==null)return;
        JSONObject event=new JSONObject();
        try{
            event.put("type","typing");
            event.put("chat_id",chatId);
            realtimeSocket.send(event.toString());
        }catch(JSONException ignored){}
    }

    private void handleRealtimeEvent(String raw){
        try{
            JSONObject event=new JSONObject(raw);
            String type=event.optString("type","");
            long chatId=event.optLong("chat_id",0);
            runOnUiThread(()->{
                if("message.created".equals(type)||"message.deleted".equals(type)){
                    cachedChats=null;
                    if(currentChat==chatId&&activeMessages!=null&&activeHistory!=null){
                        refreshMessages(chatId,activeMessages,activeHistory);
                    }else if(activeChatsList!=null){
                        requestGet("/chats",result->{
                            JSONArray fresh=result.optJSONArray("chats");
                            if(fresh!=null){
                                cachedChats=fresh;
                                if(activeChatsList!=null)renderChats(activeChatsList,fresh);
                            }
                        });
                    }
                }else if("chat.read".equals(type)&&currentChat==chatId&&
                        event.optLong("user_id",0)==activePeerID&&activePeerID>0){
                    activePeerReadCursor=Math.max(activePeerReadCursor,event.optLong("message_id",0));
                    if(activeMessages!=null){
                        for(int i=0;i<activeMessages.getChildCount();i++){
                            View row=activeMessages.getChildAt(i);
                            if(!(row instanceof LinearLayout))continue;
                            LinearLayout messageRow=(LinearLayout)row;
                            for(int j=0;j<messageRow.getChildCount();j++){
                                View child=messageRow.getChildAt(j);
                                if(child instanceof TextView&&child.getTag() instanceof Long){
                                    long messageId=(Long)child.getTag();
                                    if(messageId<=activePeerReadCursor){
                                        ((TextView)child).setText("✓✓ Прочитано");
                                        ((TextView)child).setTextColor(0xFF8FB4FF);
                                    }
                                }
                            }
                        }
                    }
                }else if("typing".equals(type)&&currentChat==chatId&&activeChatStatus!=null){
                    activeChatStatus.setText("печатает…");
                    uiHandler.postDelayed(()->{
                        if(currentChat==chatId&&activeChatStatus!=null)
                            activeChatStatus.setText(activeChatSubtitle);
                    },2500);
                }
            });
        }catch(JSONException ignored){}
    }

    @Override protected void onActivityResult(int requestCode,int resultCode,Intent data){
        super.onActivityResult(requestCode,resultCode,data);
        if(requestCode!=REQUEST_PICK_CHAT_MEDIA||resultCode!=RESULT_OK||data==null)return;
        Uri uri=data.getData();
        if(uri==null||pendingMediaChatId<=0)return;
        uploadChatMedia(pendingMediaChatId,uri,pendingMediaMessages,pendingMediaHistory);
    }

    private void uploadChatMedia(long chatId,Uri uri,LinearLayout messages,ScrollView history){
        io.execute(()->{
            HttpURLConnection connection=null;
            int status=0;
            JSONObject response=new JSONObject();
            try{
                String boundary="----kentgram-"+UUID.randomUUID().toString().replace("-","");
                String fileName=queryDisplayName(uri);
                String contentType=getContentResolver().getType(uri);
                if(contentType==null)contentType="application/octet-stream";
                connection=(HttpURLConnection)new URL(API+"/chats/"+chatId+"/media").openConnection();
                connection.setRequestMethod("POST");
                connection.setConnectTimeout(15000);
                connection.setReadTimeout(60000);
                connection.setDoOutput(true);
                connection.setChunkedStreamingMode(8192);
                connection.setRequestProperty("Authorization","Bearer "+token);
                connection.setRequestProperty("Accept","application/json");
                connection.setRequestProperty("Content-Type","multipart/form-data; boundary="+boundary);
                try(OutputStream output=connection.getOutputStream()){
                    writeMultipart(output,"--"+boundary+"\r\nContent-Disposition: form-data; name=\"caption\"\r\n\r\n\r\n");
                    String safeName=fileName.replace("\\","_").replace("\"","_").replace("\r","").replace("\n","");
                    writeMultipart(output,"--"+boundary+"\r\nContent-Disposition: form-data; name=\"file\"; filename=\""+safeName+"\"\r\nContent-Type: "+contentType+"\r\n\r\n");
                    try(InputStream input=getContentResolver().openInputStream(uri)){
                        if(input==null)throw new IOException("Unable to open selected file");
                        byte[] buffer=new byte[8192];
                        int count;
                        while((count=input.read(buffer))!=-1)output.write(buffer,0,count);
                    }
                    writeMultipart(output,"\r\n--"+boundary+"--\r\n");
                }
                status=connection.getResponseCode();
                InputStream input=status>=200&&status<300?connection.getInputStream():connection.getErrorStream();
                if(input!=null){
                    ByteArrayOutputStream bytes=new ByteArrayOutputStream();
                    byte[] buffer=new byte[4096];
                    int count;
                    while((count=input.read(buffer))!=-1)bytes.write(buffer,0,count);
                    try{response=new JSONObject(bytes.toString("UTF-8"));}catch(JSONException ignored){}
                    input.close();
                }
            }catch(Exception error){
                try{response.put("error","media_upload_failed");}catch(JSONException ignored){}
            }finally{
                if(connection!=null)connection.disconnect();
            }
            final int resultCode=status;
            final JSONObject result=response;
            runOnUiThread(()->{
                if(resultCode>=200&&resultCode<300){
                    if(currentChat==chatId&&messages!=null&&history!=null)refreshMessages(chatId,messages,history);
                    toast("Файл отправлен");
                }else{
                    toast(result.optString("error","Не удалось отправить файл")+" ("+resultCode+")");
                }
                pendingMediaChatId=0;
                pendingMediaMessages=null;
                pendingMediaHistory=null;
            });
        });
    }

    private String queryDisplayName(Uri uri){
        try(Cursor cursor=getContentResolver().query(uri,null,null,null,null)){
            if(cursor!=null&&cursor.moveToFirst()){
                int column=cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                if(column>=0)return cursor.getString(column);
            }
        }catch(Exception ignored){}
        return "attachment";
    }

    private void writeMultipart(OutputStream output,String value)throws IOException{
        output.write(value.getBytes(StandardCharsets.UTF_8));
    }

    private interface Done{void accept(JSONObject value);}
    private void requestGet(String path,Done done){request(path,null,true,done);}
    private interface Failed{void accept(JSONObject value,int status);}
    private void request(String path,JSONObject body,boolean auth,Done done){
        request(path,body,auth,done,null);
    }
    private void request(String path,JSONObject body,boolean auth,Done done,Failed failed){
        requestMethod(body==null?"GET":"POST",path,body,auth,done,failed);
    }
    private void requestMethod(String method,String path,JSONObject body,boolean auth,
                               Done done,Failed failed){
        io.execute(()->{
            JSONObject out=new JSONObject();
            int status=0;
            try{
                HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();
                c.setRequestMethod(method);
                c.setConnectTimeout(12000);
                c.setReadTimeout(12000);
                c.setRequestProperty("Accept","application/json");
                if(auth&&!token.isEmpty())
                    c.setRequestProperty("Authorization","Bearer "+token);
                if(body!=null){
                    c.setDoOutput(true);
                    c.setRequestProperty("Content-Type","application/json");
                    try(OutputStream stream=c.getOutputStream()){
                        stream.write(body.toString().getBytes(StandardCharsets.UTF_8));
                    }
                }
                status=c.getResponseCode();
                InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();
                if(in!=null){
                    ByteArrayOutputStream bytes=new ByteArrayOutputStream();
                    byte[] buffer=new byte[4096];
                    int count;
                    while((count=in.read(buffer))>0)bytes.write(buffer,0,count);
                    try{
                        out=new JSONObject(bytes.toString("UTF-8"));
                    }catch(JSONException parseError){
                        out=new JSONObject();
                        out.put("error",status==404?"endpoint_not_found":
                                status>=500?"server_error":"http_error");
                    }
                }
                c.disconnect();
            }catch(Exception e){
                try{out.put("error","connection_failed");}
                catch(Exception ignored){}
            }

            final JSONObject result=out;
            final int code=status;
            runOnUiThread(()->{
                if(code>=200&&code<300){
                    if(done!=null)done.accept(result);
                    return;
                }
                if(code==401&&auth){
                    prefs.edit().clear().apply();
                    token="";
                    showAuth();
                    toast("Войдите снова");
                    return;
                }
                if(failed!=null){
                    failed.accept(result,code);
                    return;
                }
                KentDialog.showMessage(this,
                        code==404?"Маршрут не найден":"Не удалось выполнить запрос",
                        result.optString("error","Ошибка соединения")+" ("+code+")",
                        "Понятно",null);
            });
        });
    }
    private void requestDelete(String path,Done done){
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
                    KentDialog.showMessage(this,"Не удалось удалить сообщение",
                            result.optString("error","Ошибка соединения")+" ("+code+")","Понятно",null);
                }
            });
        });
    }
    private void loadChatMedia(ImageView target,String url){
        if(target==null||url==null||url.trim().isEmpty())return;
        Bitmap cached=mediaCache.get(url);
        if(cached!=null){target.setImageBitmap(cached);return;}
        ArrayList<ImageView> waiting=mediaTargets.computeIfAbsent(url,key->new ArrayList<>());
        waiting.add(target);
        if(!mediaLoading.add(url))return;
        io.execute(()->{
            HttpURLConnection connection=null;
            Bitmap bitmap=null;
            try{
                connection=(HttpURLConnection)new URL(url).openConnection();
                connection.setConnectTimeout(10000);
                connection.setReadTimeout(15000);
                connection.setRequestProperty("Authorization","Bearer "+token);
                try(InputStream input=connection.getInputStream()){
                    bitmap=BitmapFactory.decodeStream(input);
                }
            }catch(Exception ignored){}
            finally{if(connection!=null)connection.disconnect();}
            final Bitmap loaded=bitmap;
            runOnUiThread(()->{
                mediaLoading.remove(url);
                ArrayList<ImageView> targets=mediaTargets.remove(url);
                if(loaded!=null){
                    mediaCache.put(url,loaded);
                    if(targets!=null)for(ImageView view:targets)
                        if(!isFinishing()&&view.getWindowToken()!=null)view.setImageBitmap(loaded);
                }
            });
        });
    }

    private void showChatImage(String url){
        ImageView image=new ImageView(this);
        image.setAdjustViewBounds(true);
        image.setScaleType(ImageView.ScaleType.FIT_CENTER);
        image.setBackgroundColor(Color.BLACK);
        loadChatMedia(image,url);
        new AlertDialog.Builder(this).setView(image).setPositiveButton("Закрыть",null).show();
    }

    private String formatBytes(long bytes){
        if(bytes<1024)return bytes+" Б";
        if(bytes<1024*1024)return String.format(java.util.Locale.ROOT,"%.1f КБ",bytes/1024.0);
        return String.format(java.util.Locale.ROOT,"%.1f МБ",bytes/(1024.0*1024.0));
    }

    private void loadChatAvatar(ImageView target,String url){
        if(target==null||url==null||url.trim().isEmpty())return;
        Bitmap cached=avatarCache.get(url);
        if(cached!=null){target.setImageBitmap(cached);return;}
        ArrayList<ImageView> targets=avatarTargets.computeIfAbsent(url,key->new ArrayList<>());
        targets.add(target);
        if(!avatarLoading.add(url))return;
        io.execute(()->{
            HttpURLConnection connection=null;
            Bitmap bitmap=null;
            try{
                connection=(HttpURLConnection)new URL(url).openConnection();
                connection.setConnectTimeout(6000);connection.setReadTimeout(6000);
                connection.setInstanceFollowRedirects(true);
                try(InputStream input=connection.getInputStream()){bitmap=BitmapFactory.decodeStream(input);}
            }catch(Exception ignored){}
            finally{if(connection!=null)connection.disconnect();}
            final Bitmap loaded=bitmap;
            runOnUiThread(()->{
                avatarLoading.remove(url);
                ArrayList<ImageView> waiting=avatarTargets.remove(url);
                if(loaded!=null){
                    avatarCache.put(url,loaded);
                    if(waiting!=null)for(ImageView view:waiting)
                        if(!isFinishing()&&view.getWindowToken()!=null)view.setImageBitmap(loaded);
                }
            });
        });
    }
    private String enc(String s){try{return URLEncoder.encode(s,"UTF-8");}catch(Exception e){return s;}}
    private EditText field(String hint){EditText e=new EditText(this);e.setTextColor(TEXT);e.setHintTextColor(0xFF778397);e.setHint(hint);e.setTextSize(14);e.setPadding(dp(14),dp(10),dp(14),dp(10));e.setBackground(round(PANEL,15));e.setSingleLine(false);LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(9);e.setLayoutParams(p);return e;}
    private Button button(String s){Button b=new Button(this);b.setText(s);b.setTextColor(Color.WHITE);b.setTextSize(14);b.setTypeface(null,Typeface.BOLD);b.setAllCaps(false);b.setMinHeight(dp(48));b.setPadding(dp(18),dp(8),dp(18),dp(8));b.setBackground(round(ACCENT,28));b.setElevation(dp(2));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(8);b.setLayoutParams(p);return b;}
    private TextView label(String s,int size,int color,boolean bold){TextView t=new TextView(this);t.setText(s);t.setTextSize(size);t.setTextColor(color);if(bold)t.setTypeface(null,Typeface.BOLD);return t;}
    private android.graphics.drawable.GradientDrawable round(int color,int radius){android.graphics.drawable.GradientDrawable d=new android.graphics.drawable.GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));return d;}
    private int dp(float n){return (int)(n*getResources().getDisplayMetrics().density+0.5f);}
    private void toast(String s){KentDialog.showToast(this,s);}
    @Override public void onBackPressed(){
        if(onNewChatScreen||onChatScreen){showChats();return;}
        super.onBackPressed();
    }
    @Override protected void onDestroy(){
        if(clearHighlightTask!=null)uiHandler.removeCallbacks(clearHighlightTask);
        stopRealtime();
        super.onDestroy();io.shutdown();}
}
