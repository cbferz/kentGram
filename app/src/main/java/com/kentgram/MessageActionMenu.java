package com.kentgram;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.PopupWindow;
import android.widget.TextView;
import java.util.Set;

final class MessageActionMenu {
    /** Quick reactions offered in the menu; any of them is accepted by the server. */
    static final String[] REACTIONS = {"\uD83D\uDC4D", "\u2764\uFE0F", "\uD83D\uDE02", "\uD83D\uDE2E",
            "\uD83D\uDE22", "\uD83D\uDD25", "\uD83C\uDF89", "\uD83D\uDC4E"};

    interface ReactionHandler { void onReact(String emoji, boolean alreadyMine); }

    private MessageActionMenu() {}

    static void show(Activity activity, View anchor, String message,
                     boolean canDelete, Runnable onDelete, Runnable onReply) {
        show(activity, anchor, message, canDelete, null, onDelete, onReply, null);
    }

    /**
     * @param mine   emojis the current user already reacted with (highlighted), may be null
     * @param onReact called with the tapped emoji; when null the reaction row is hidden
     */
    static void show(Activity activity, View anchor, String message,
                     boolean canDelete, Set<String> mine, Runnable onDelete, Runnable onReply,
                     ReactionHandler onReact) {
        if (activity == null || anchor == null || message == null) return;

        View decor = activity.getWindow().getDecorView();
        FrameLayout frame = decor instanceof FrameLayout ? (FrameLayout) decor : null;
        View dim = new View(activity);
        dim.setBackgroundColor(0x99070B12);
        if (frame != null) {
            frame.addView(dim, new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        }

        LinearLayout box = new LinearLayout(activity);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setPadding(dp(activity, 7), dp(activity, 7), dp(activity, 7), dp(activity, 7));
        LinearLayout menu = new LinearLayout(activity);
        menu.setOrientation(LinearLayout.HORIZONTAL);
        menu.setGravity(Gravity.CENTER);
        GradientDrawable background = new GradientDrawable();
        background.setColor(0xFF202A38);
        background.setCornerRadius(dp(activity, 18));
        background.setStroke(dp(activity, 1), 0xFF35465D);
        box.setBackground(background);

        int screenW = activity.getResources().getDisplayMetrics().widthPixels;
        int popupW = Math.min(dp(activity, onReact != null ? 330 : (canDelete ? 300 : 190)), screenW - dp(activity, 24));
        boolean hasRow = onReact != null;
        int popupH = dp(activity, 14 + 56 + (hasRow ? 52 : 0));
        PopupWindow popup = new PopupWindow(box, popupW, popupH, true);
        popup.setBackgroundDrawable(background);
        popup.setOutsideTouchable(true);
        popup.setElevation(dp(activity, 16));

        if (hasRow) {
            LinearLayout row = new LinearLayout(activity);
            row.setOrientation(LinearLayout.HORIZONTAL);
            row.setGravity(Gravity.CENTER);
            for (String emoji : REACTIONS) {
                final boolean isMine = mine != null && mine.contains(emoji);
                TextView cell = new TextView(activity);
                cell.setText(emoji);
                cell.setTextSize(22);
                cell.setGravity(Gravity.CENTER);
                cell.setContentDescription("Реакция " + emoji);
                if (isMine) {
                    GradientDrawable hl = new GradientDrawable();
                    hl.setColor(0xFF2D4F7A);
                    hl.setCornerRadius(dp(activity, 14));
                    cell.setBackground(hl);
                }
                cell.setOnClickListener(v -> { popup.dismiss(); onReact.onReact(emoji, isMine); });
                LinearLayout.LayoutParams cp = new LinearLayout.LayoutParams(0, dp(activity, 44), 1);
                cp.setMargins(dp(activity, 1), 0, dp(activity, 1), 0);
                row.addView(cell, cp);
            }
            box.addView(row, new LinearLayout.LayoutParams(-1, dp(activity, 48)));
        }
        box.addView(menu, new LinearLayout.LayoutParams(-1, dp(activity, 56)));

        TextView copy = action(activity, "Копировать", 0xFFF4F7FB);
        menu.addView(copy, new LinearLayout.LayoutParams(0, -1, 1));
        copy.setOnClickListener(v -> {
            ClipboardManager clipboard = (ClipboardManager) activity.getSystemService(Context.CLIPBOARD_SERVICE);
            if (clipboard != null) {
                clipboard.setPrimaryClip(ClipData.newPlainText("kentGram message", message));
                android.widget.Toast.makeText(activity, "Сообщение скопировано",
                        android.widget.Toast.LENGTH_SHORT).show();
            }
            popup.dismiss();
        });

        TextView reply = action(activity, "Ответить", 0xFF76A7FF);
        menu.addView(reply, new LinearLayout.LayoutParams(0, -1, 1));
        reply.setOnClickListener(v -> { popup.dismiss(); if (onReply != null) onReply.run(); });

        if (canDelete) {
            View separator = new View(activity);
            separator.setBackgroundColor(0xFF35465D);
            menu.addView(separator, new LinearLayout.LayoutParams(dp(activity, 1), dp(activity, 28)));
            TextView delete = action(activity, "Удалить", 0xFFFF8995);
            menu.addView(delete, new LinearLayout.LayoutParams(0, -1, 1));
            delete.setOnClickListener(v -> {
                popup.dismiss();
                if (onDelete != null) onDelete.run();
            });
        }

        popup.setOnDismissListener(() -> {
            if (frame != null && dim.getParent() == frame) frame.removeView(dim);
        });

        int[] location = new int[2];
        anchor.getLocationOnScreen(location);
        int x = Math.max(dp(activity, 12),
                Math.min(location[0] + anchor.getWidth() / 2 - popupW / 2, screenW - popupW - dp(activity, 12)));
        int y = Math.max(dp(activity, 28), location[1] - popupH - dp(activity, 6));
        popup.showAtLocation(decor, Gravity.TOP | Gravity.LEFT, x, y);
        if (frame != null) dim.setOnClickListener(v -> popup.dismiss());
        dim.bringToFront();
        popup.getContentView().bringToFront();
    }

    private static TextView action(Activity activity, String title, int color) {
        TextView view = new TextView(activity);
        view.setText(title);
        view.setTextColor(color);
        view.setTextSize(14);
        view.setTypeface(null, Typeface.BOLD);
        view.setGravity(Gravity.CENTER);
        view.setPadding(dp(activity, 10), 0, dp(activity, 10), 0);
        return view;
    }

    private static int dp(Activity activity, float value) {
        return (int) (value * activity.getResources().getDisplayMetrics().density + 0.5f);
    }
}
