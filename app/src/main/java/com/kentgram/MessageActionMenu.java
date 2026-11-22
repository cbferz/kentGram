package com.kentgram;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.PopupWindow;
import android.widget.TextView;

final class MessageActionMenu {
    interface ReactionCallback { void onReaction(String emoji); }

    private static final String[] REACTIONS={"👍","❤️","😂","😮","😢","🔥","👏","🎉"};

    private MessageActionMenu() {}

    static void show(Activity activity, View anchor, String message,
                     boolean canDelete, Runnable onDelete, Runnable onReply,
                     Runnable onEdit, ReactionCallback onReaction) {
        if (activity == null || anchor == null || message == null) return;

        View decor = activity.getWindow().getDecorView();
        FrameLayout frame = decor instanceof FrameLayout ? (FrameLayout) decor : null;
        View dim = new View(activity);
        dim.setBackgroundColor(0x99070B12);
        if (frame != null) {
            frame.addView(dim, new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        }

        LinearLayout panel = new LinearLayout(activity);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setPadding(dp(activity, 9), dp(activity, 8), dp(activity, 9), dp(activity, 7));
        GradientDrawable background = new GradientDrawable();
        background.setColor(0xFF1A2230);
        background.setCornerRadius(dp(activity, 20));
        background.setStroke(dp(activity, 1), 0xFF3A4A64);
        panel.setBackground(background);

        int width = Math.max(dp(activity, 280),
                Math.min(decor.getWidth() - dp(activity, 24), dp(activity, 390)));
        PopupWindow popup = new PopupWindow(panel, width, dp(activity, 104), true);
        popup.setBackgroundDrawable(background);
        popup.setOutsideTouchable(true);
        popup.setElevation(dp(activity, 18));

        LinearLayout reactions = new LinearLayout(activity);
        reactions.setGravity(Gravity.CENTER);
        reactions.setOrientation(LinearLayout.HORIZONTAL);
        for (String emoji : REACTIONS) {
            TextView item = new TextView(activity);
            item.setText(emoji);
            item.setTextSize(20);
            item.setGravity(Gravity.CENTER);
            item.setBackground(pill(activity, 0xFF222D3D, 0xFF34445A));
            LinearLayout.LayoutParams itemParams = new LinearLayout.LayoutParams(0, dp(activity, 38), 1);
            itemParams.setMargins(dp(activity, 2), 0, dp(activity, 2), 0);
            reactions.addView(item, itemParams);
            item.setOnClickListener(v -> {
                popup.dismiss();
                if (onReaction != null) onReaction.onReaction(emoji);
            });
        }
        panel.addView(reactions, new LinearLayout.LayoutParams(-1, dp(activity, 40)));

        View divider = new View(activity);
        divider.setBackgroundColor(0xFF303D50);
        LinearLayout.LayoutParams dividerParams = new LinearLayout.LayoutParams(-1, dp(activity, 1));
        dividerParams.setMargins(dp(activity, 3), dp(activity, 5), dp(activity, 3), dp(activity, 3));
        panel.addView(divider, dividerParams);

        LinearLayout actions = new LinearLayout(activity);
        actions.setGravity(Gravity.CENTER);
        actions.setOrientation(LinearLayout.HORIZONTAL);

        TextView copy = action(activity, "Копировать", 0xFFE1E7F0);
        actions.addView(copy, new LinearLayout.LayoutParams(0, dp(activity, 39), 1));
        copy.setOnClickListener(v -> {
            ClipboardManager clipboard = (ClipboardManager) activity.getSystemService(Context.CLIPBOARD_SERVICE);
            if (clipboard != null) {
                clipboard.setPrimaryClip(ClipData.newPlainText("kentGram message", message));
                android.widget.Toast.makeText(activity, "Сообщение скопировано",
                        android.widget.Toast.LENGTH_SHORT).show();
            }
            popup.dismiss();
        });

        TextView reply = action(activity, "Ответить", 0xFF9AB5FF);
        actions.addView(reply, new LinearLayout.LayoutParams(0, dp(activity, 39), 1));
        reply.setOnClickListener(v -> { popup.dismiss(); if (onReply != null) onReply.run(); });

        if (canDelete) {
            TextView edit = action(activity, "Изменить", 0xFF9AB5FF);
            actions.addView(edit, new LinearLayout.LayoutParams(0, dp(activity, 39), 1));
            edit.setOnClickListener(v -> {
                popup.dismiss();
                if (onEdit != null) onEdit.run();
            });

            TextView delete = action(activity, "Удалить", 0xFFFF8995);
            actions.addView(delete, new LinearLayout.LayoutParams(0, dp(activity, 39), 1));
            delete.setOnClickListener(v -> {
                popup.dismiss();
                if (onDelete != null) onDelete.run();
            });
        }
        panel.addView(actions, new LinearLayout.LayoutParams(-1, dp(activity, 39)));

        popup.setOnDismissListener(() -> {
            if (frame != null && dim.getParent() == frame) frame.removeView(dim);
        });

        int[] location = new int[2];
        anchor.getLocationOnScreen(location);
        int x = Math.max(dp(activity, 12), (decor.getWidth() - width) / 2);
        int y = Math.max(dp(activity, 28), location[1] - dp(activity, 116));
        popup.showAtLocation(decor, Gravity.TOP | Gravity.LEFT, x, y);
        if (frame != null) dim.setOnClickListener(v -> popup.dismiss());
        dim.bringToFront();
        popup.getContentView().bringToFront();
    }

    private static TextView action(Activity activity, String title, int color) {
        TextView view = new TextView(activity);
        view.setText(title);
        view.setTextColor(color);
        view.setTextSize(12);
        view.setTypeface(null, Typeface.BOLD);
        view.setGravity(Gravity.CENTER);
        view.setPadding(dp(activity, 3), 0, dp(activity, 3), 0);
        return view;
    }

    private static GradientDrawable pill(Activity activity, int color, int stroke) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(color);
        drawable.setCornerRadius(dp(activity, 13));
        drawable.setStroke(dp(activity, 1), stroke);
        return drawable;
    }

    private static int dp(Activity activity, float value) {
        return (int) (value * activity.getResources().getDisplayMetrics().density + 0.5f);
    }
}
