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
import android.widget.Toast;

final class MessageActionMenu {
    interface ReactionListener {
        void onReaction(String emoji);
    }

    private MessageActionMenu() {}

    static void show(Activity activity, View anchor, String message,
                     boolean canDelete, boolean canEdit,
                     Runnable onDelete, Runnable onReply,
                     Runnable onEdit, ReactionListener onReaction,
                     Runnable onForward, Runnable onPin) {
        if (activity == null || anchor == null || message == null) return;

        final String[] reactions = {
                "👍", "❤️", "😂", "😮", "😢", "🔥", "👏", "🎉"
        };

        View decor = activity.getWindow().getDecorView();
        FrameLayout frame = decor instanceof FrameLayout ? (FrameLayout) decor : null;

        View dim = new View(activity);
        dim.setBackgroundColor(0x99070B12);
        if (frame != null) {
            frame.addView(dim, new FrameLayout.LayoutParams(-1, -1));
        }

        LinearLayout container = new LinearLayout(activity);
        container.setOrientation(LinearLayout.VERTICAL);
        container.setPadding(dp(activity, 10), dp(activity, 9),
                dp(activity, 10), dp(activity, 9));

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(0xFF171F2C);
        bg.setCornerRadius(dp(activity, 20));
        bg.setStroke(dp(activity, 1), 0xFF34445D);
        container.setBackground(bg);

        final PopupWindow[] popupRef = new PopupWindow[1];
        LinearLayout reactionRow = new LinearLayout(activity);
        reactionRow.setGravity(Gravity.CENTER);
        reactionRow.setOrientation(LinearLayout.HORIZONTAL);

        for (String emoji : reactions) {
            TextView item = new TextView(activity);
            item.setText(emoji);
            item.setTextSize(20);
            item.setGravity(Gravity.CENTER);
            item.setBackground(circle(activity, 0xFF263348));
            LinearLayout.LayoutParams rp = new LinearLayout.LayoutParams(
                    0, dp(activity, 42), 1f);
            rp.setMargins(dp(activity, 2), 0, dp(activity, 2), 0);
            reactionRow.addView(item, rp);
            item.setOnClickListener(v -> {
                if (onReaction != null) onReaction.onReaction(emoji);
                if (popupRef[0] != null) popupRef[0].dismiss();
            });
        }

        container.addView(reactionRow);

        View separator = new View(activity);
        separator.setBackgroundColor(0xFF34445D);
        LinearLayout.LayoutParams sp = new LinearLayout.LayoutParams(-1, dp(activity, 1));
        sp.setMargins(dp(activity, 2), dp(activity, 9), dp(activity, 2), dp(activity, 5));
        container.addView(separator, sp);

        LinearLayout actions = new LinearLayout(activity);
        actions.setGravity(Gravity.CENTER);
        actions.setOrientation(LinearLayout.HORIZONTAL);
        container.addView(actions);

        PopupWindow popup = new PopupWindow(container,
                Math.min(dp(activity, 340),
                        activity.getResources().getDisplayMetrics().widthPixels - dp(activity, 24)),
                -2, true);
        popupRef[0] = popup;
        popup.setBackgroundDrawable(bg);
        popup.setOutsideTouchable(true);
        popup.setElevation(dp(activity, 18));

        addAction(activity, actions, "⧉", "Копировать", 0xFFE7EDF7, () -> {
            ClipboardManager clipboard = (ClipboardManager)
                    activity.getSystemService(Context.CLIPBOARD_SERVICE);
            if (clipboard != null) {
                clipboard.setPrimaryClip(ClipData.newPlainText("kentGram message", message));
                Toast.makeText(activity, "Сообщение скопировано",
                        Toast.LENGTH_SHORT).show();
            }
            popup.dismiss();
        });

        addAction(activity, actions, "↩", "Ответить", 0xFF9AB5FF, () -> {
            popup.dismiss();
            if (onReply != null) onReply.run();
        });

        addAction(activity, actions, "➦", "Переслать", 0xFF9AB5FF, () -> {
            popup.dismiss();
            if (onForward != null) onForward.run();
        });

        addAction(activity, actions, "⌖", "Закрепить", 0xFF9AB5FF, () -> {
            popup.dismiss();
            if (onPin != null) onPin.run();
        });

        if (canEdit) {
            addAction(activity, actions, "✎", "Изменить", 0xFF9AB5FF, () -> {
                popup.dismiss();
                if (onEdit != null) onEdit.run();
            });
        }

        if (canDelete) {
            addAction(activity, actions, "⌫", "Удалить", 0xFFFF8995, () -> {
                popup.dismiss();
                if (onDelete != null) onDelete.run();
            });
        }

        popup.setOnDismissListener(() -> {
            if (frame != null && dim.getParent() == frame) frame.removeView(dim);
        });

        int[] pos = new int[2];
        anchor.getLocationOnScreen(pos);
        int screenWidth = activity.getResources().getDisplayMetrics().widthPixels;
        int width = Math.min(dp(activity, 340), screenWidth - dp(activity, 24));
        int x = Math.max(dp(activity, 12),
                Math.min(screenWidth - width - dp(activity, 12),
                        pos[0] + anchor.getWidth() / 2 - width / 2));
        int y = Math.max(dp(activity, 28), pos[1] - dp(activity, 150));

        popup.showAtLocation(decor, Gravity.TOP | Gravity.LEFT, x, y);

        if (frame != null) dim.setOnClickListener(v -> popup.dismiss());
        dim.bringToFront();
        popup.getContentView().bringToFront();

        // Keep a reference for reaction callbacks.
    }

    private static void addAction(Activity activity, LinearLayout parent,
                                  String symbol, String title, int color,
                                  Runnable callback) {
        LinearLayout item = new LinearLayout(activity);
        item.setOrientation(LinearLayout.VERTICAL);
        item.setGravity(Gravity.CENTER);

        TextView icon = new TextView(activity);
        icon.setText(symbol);
        icon.setTextColor(color);
        icon.setTextSize(21);
        icon.setGravity(Gravity.CENTER);

        TextView label = new TextView(activity);
        label.setText(title);
        label.setTextColor(color);
        label.setTextSize(10);
        label.setTypeface(null, Typeface.BOLD);
        label.setGravity(Gravity.CENTER);

        item.addView(icon);
        item.addView(label);

        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(
                0, dp(activity, 55), 1f);
        item.setLayoutParams(p);
        parent.addView(item);
        item.setOnClickListener(v -> callback.run());
    }

    private static GradientDrawable circle(Activity activity, int color) {
        GradientDrawable d = new GradientDrawable();
        d.setShape(GradientDrawable.OVAL);
        d.setColor(color);
        return d;
    }

    private static int dp(Activity activity, float value) {
        return (int) (value * activity.getResources().getDisplayMetrics().density + 0.5f);
    }
}
