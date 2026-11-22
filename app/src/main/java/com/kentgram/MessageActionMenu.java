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

final class MessageActionMenu {
    private MessageActionMenu() {}

    static void show(Activity activity, View anchor, String message,
                     boolean canDelete, Runnable onDelete, Runnable onReply) {
        if (activity == null || anchor == null || message == null) return;

        View decor = activity.getWindow().getDecorView();
        FrameLayout frame = decor instanceof FrameLayout ? (FrameLayout) decor : null;
        View dim = new View(activity);
        dim.setBackgroundColor(0x99070B12);
        if (frame != null) {
            frame.addView(dim, new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        }

        LinearLayout menu = new LinearLayout(activity);
        menu.setOrientation(LinearLayout.HORIZONTAL);
        menu.setGravity(Gravity.CENTER);
        menu.setPadding(dp(activity, 7), dp(activity, 7), dp(activity, 7), dp(activity, 7));
        GradientDrawable background = new GradientDrawable();
        background.setColor(0xFF1A2230);
        background.setCornerRadius(dp(activity, 18));
        background.setStroke(dp(activity, 1), 0xFF3A4A64);
        menu.setBackground(background);

        PopupWindow popup = new PopupWindow(menu, dp(activity, canDelete ? 300 : 190),
                dp(activity, 56), true);
        popup.setBackgroundDrawable(background);
        popup.setOutsideTouchable(true);
        popup.setElevation(dp(activity, 16));

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

        TextView reply = action(activity, "Ответить", 0xFF9AB5FF);
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
                location[0] + anchor.getWidth() / 2 - dp(activity, canDelete ? 150 : 95));
        int y = Math.max(dp(activity, 28), location[1] - dp(activity, 68));
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
