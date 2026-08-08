#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成 ghdeb 演示 GIF（终端动画，纯 Pillow 绘制，无外部依赖）。
用法: python3 scripts/demo_gif.py  →  输出 docs/demo/ghdeb-demo.gif
"""
from PIL import Image, ImageDraw, ImageFont

# ---------- 配色（One Dark 风格） ----------
BG      = (40, 44, 52)    # #282c34
BG_HEAD = (33, 37, 43)
TXT     = (171, 178, 191)
PROMPT  = (97, 175, 239)
CMD     = (230, 230, 230)
GREEN   = (152, 195, 121)
YELLOW  = (229, 192, 123)
DIM     = (125, 135, 150)
TITLE   = (255, 200, 120)
RED,GRN,YLW = (255,95,86),(39,201,63),(255,189,46)

def _load_font(size):
    """优先加载含中文字形的等宽字体，找不到再退回 DejaVu。"""
    for path, index in (
        ("/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", 7),  # Noto Sans Mono CJK SC
        ("/usr/share/fonts/truetype/wqy/wqy-microhei.ttc", 0),          # 文泉驿等宽微米黑
    ):
        try:
            return ImageFont.truetype(path, size, index=index)
        except Exception:
            continue
    return ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", size)

FONT   = _load_font(15)
FONT_S = _load_font(13)   # 表格小字

COL, CELL = 68, 9
ROW_H, PAD, HEAD_H, ROWS = 21, 16, 26, 26
W = PAD*2 + COL*CELL
H = PAD*2 + HEAD_H + ROWS*ROW_H

def draw_window(draw):
    draw.rounded_rectangle([0,0,W,H], radius=10, fill=BG)
    draw.rectangle([0,0,W,HEAD_H], fill=BG_HEAD)
    for x,c in [(12,RED),(32,YLW),(52,GRN)]:
        draw.ellipse([x,8,x+13,21], fill=c)
    draw.text((76,6),"ghdeb  —  from GitHub Releases to your system",font=FONT,fill=DIM)

def ry(r): return PAD+HEAD_H+4+r*ROW_H
def put(draw,r,text,color=TXT,font=FONT):
    draw.text((PAD,ry(r)),text,font=font,fill=color)

def main():
    # 时间轴（帧, fps=10）：事件 -> 追加的行
    # 每步: (起始帧, 命令字符数, 提示行) 用打字机; 输出行在命令结束后追加
    seq = []   # (start_frame, cmd, outputs:list[(row_offset_after_cmd, text, color)])

    outputs_install = [
        (0,"  downloading sharkdp/bat_v0.26.1_amd64.deb",YELLOW),
        (2,"  ✓ asset verified",GREEN),
        (4,"  ✓ bat v0.26.1 installed",GREEN),
    ]
    outputs_update = [
        (0,"  checking 27 packages for latest versions ...",DIM),
        (2,"  ✓ snapshot refreshed → /var/cache/ghdeb/cache.json",GREEN),
    ]
    table = [
        (0,"  名称            仓库/URL             已装      最新      状态",PROMPT),
        (1,"  balena-etcher   balena-io/etcher      -         v2.1.6    未装",TXT),
        (2,"  bat             sharkdp/bat           0.26.1    v0.26.1   ✓",GREEN),
        (3,"  caddy           caddyserver/caddy     -         v2.11.4   未装",TXT),
        (4,"  gh              cli/cli               2.68.1    2.73.2    可升级",YELLOW),
        (5,"  rustdesk        rustdesk/rustdesk     1.5.2     1.5.3     可升级",YELLOW),
        (7,"  ✓ 27 packages · cache fresh · 5 architectures",GREEN),
    ]

    fps = 10
    # 构造时间轴
    TL = []   # (start_frame, cmd, outputs)
    f = 15
    TL.append((f,"ghdeb install bat",outputs_install));  f += 28
    TL.append((f,"ghdeb update",outputs_update));        f += 22
    TL.append((f,"ghdeb list",table));                   f += 40
    TOTAL = f + 8

    frames=[]
    for frame in range(TOTAL):
        img=Image.new("RGB",(W,H),(0,0,0)); draw=ImageDraw.Draw(img)
        draw_window(draw)
        row=2
        # 标题两行
        put(draw,row,"  ghdeb  ▸  one command for all your .deb packages",TITLE); row+=1
        put(draw,row,"  bat · fd · ripgrep · gh · rustdesk ...",DIM); row+=2

        # 每个时间轴步骤
        for (s,cmd,outs) in TL:
            if frame < s: continue
            # 提示符 + 命令(打字机)
            n = min(len(cmd), max(0,(frame-s)//2))
            put(draw,row,"  $ "+cmd[:n],color=CMD); 
            done = n==len(cmd)
            # 命令完成后再追加输出
            if done:
                rr = row+1
                for (off,text,color) in outs:
                    # 进度条特例：install 第0行显示进度动画
                    if "downloading" in text and frame < s+20:
                        pct=min(100,(frame-s-2)*5)
                        b="█"*(pct//10)+"░"*(10-pct//10)
                        put(draw,rr,text+f"  [{b}] {pct:3d}%",color=color)
                    elif frame >= s+8+off*4:   # 逐条输出
                        put(draw,rr,text,color=color)
                    rr+=1
            row += 1 + len(outs)+1
            row += 1  # 空行
        frames.append(img)

    frames[0].save("docs/demo/ghdeb-demo.gif",save_all=True,append_images=frames[1:],
                   duration=1000//fps,loop=0,optimize=True)
    print(f"已生成 docs/demo/ghdeb-demo.gif: {len(frames)} 帧, {len(frames)/fps:.1f}s")

if __name__=="__main__":
    main()
