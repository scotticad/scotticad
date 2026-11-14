"""Makes the manual's screenshots (docs/manual/images) by driving a hidden
ScottiCAD through the automation API, then cropping them and drawing the
numbered callouts.

    python docs/manual/make_images.py            every image
    python docs/manual/make_images.py tour view  only those chapters' images

It needs Pillow (pip install pillow) and a built ScottiCAD: the one at the
top of the repository, or the one SCOTTICAD_EXE names. Every model in the
pictures is made here, from the shape library; nothing is opened from disk.
"""
import math
import os
import sys
import tempfile

from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "..", "python"))
from scotticad import launch  # noqa: E402

OUT = os.path.join(HERE, "images")
W, H = 1280, 800
MARK = (21, 101, 192)  # the callouts' blue


# --- helpers -------------------------------------------------------------------

def rect(cad, id):
    for c in cad.controls():
        if c["id"] == id:
            return c["rect"]
    raise KeyError(id)


def has(cad, id):
    return any(c["id"] == id for c in cad.controls())


def center(cad, id):
    x, y, w, h = rect(cad, id)
    return x + w / 2, y + h / 2


def park(cad):
    """Puts the pointer where it hovers nothing: the bottom-left corner of
    the view."""
    cad.mouse_move(6, H - 6)


def settle(cad):
    """Waits until background combining is finished."""
    import time
    for _ in range(600):
        if cad.ui_state()["background_jobs"] == 0:
            return
        time.sleep(0.05)


def grab(cad):
    settle(cad)
    path = os.path.join(tempfile.gettempdir(), "scotticad-manual-shot.png")
    cad.screenshot(path)
    img = Image.open(path).convert("RGB")
    img.load()
    return img


def font(size):
    return ImageFont.load_default(size)


def mark(img, n, at, to=None):
    """Draws callout n at `at`, with a line to `to` when given."""
    d = ImageDraw.Draw(img)
    if to is not None:
        # the line stops short, so it doesn't cover what it points at
        l = math.hypot(to[0] - at[0], to[1] - at[1])
        if l > 30:
            to = (to[0] + (at[0] - to[0]) * 9 / l, to[1] + (at[1] - to[1]) * 9 / l)
        d.line([at, to], fill=(255, 255, 255), width=4)
        d.line([at, to], fill=MARK, width=2)
        d.ellipse([to[0] - 2.5, to[1] - 2.5, to[0] + 2.5, to[1] + 2.5], fill=MARK)
    r = 12
    d.ellipse([at[0] - r - 2, at[1] - r - 2, at[0] + r + 2, at[1] + r + 2], fill=(255, 255, 255))
    d.ellipse([at[0] - r, at[1] - r, at[0] + r, at[1] + r], fill=MARK)
    d.text(at, str(n), font=font(15), fill=(255, 255, 255), anchor="mm")


def save(img, name, crop=None, marks=()):
    """Saves img as images/name.png: marks are (n, at) or (n, at, to) in
    window coordinates, drawn before cropping to crop (x, y, w, h)."""
    for m in marks:
        mark(img, *m)
    if crop:
        x, y, w, h = [int(round(v)) for v in crop]
        x, y = max(0, x), max(0, y)
        img = img.crop((x, y, min(img.width, x + w), min(img.height, y + h)))
    # a thin frame, so a crop doesn't bleed into a white page
    ImageDraw.Draw(img).rectangle([0, 0, img.width - 1, img.height - 1], outline=(200, 190, 175))
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + ".png"), optimize=True)
    print(name)


def shot(cad, name, crop=None, marks=(), keep_pointer=False):
    if not keep_pointer:
        park(cad)
    save(grab(cad), name, crop, marks)


def side_by_side(images, name, gap=12, labels=None):
    """Joins images left to right, with a caption strip under each."""
    h = max(i.height for i in images)
    strip = 26 if labels else 0
    out = Image.new("RGB", (sum(i.width for i in images) + gap * (len(images) - 1), h + strip), (255, 255, 255))
    x = 0
    d = ImageDraw.Draw(out)
    for k, i in enumerate(images):
        out.paste(i, (x, 0))
        d.rectangle([x, 0, x + i.width - 1, i.height - 1], outline=(200, 190, 175))
        if labels:
            d.text((x + i.width / 2, h + strip / 2), labels[k], font=font(15), fill=(60, 50, 40), anchor="mm")
        x += i.width + gap
    os.makedirs(OUT, exist_ok=True)
    out.save(os.path.join(OUT, name + ".png"), optimize=True)
    print(name)


def view(cad, crop):
    """A crop of the 3D view, without marks."""
    park(cad)
    x, y, w, h = crop
    return grab(cad).crop((x, y, x + w, y + h))


def drag_tile(cad, kind, x, y, z=0, drop=True, steps=8):
    """Drags a shape's tile from the panel to the point (x, y, z) mm."""
    tx, ty = center(cad, "tile-" + kind)
    px, py = cad.project(x, y, z)
    cad.mouse_down(tx, ty)
    for i in range(1, steps + 1):
        cad.mouse_move(tx + (px - tx) * i / steps, ty + (py - ty) * i / steps)
    if drop:
        cad.mouse_up(px, py)
    return px, py


def fresh(cad, target=(0, 0, 5), dist=150, yaw=None, pitch=None):
    while cad.ui_state()["popup"]:
        cad.key("esc")
    cad.set_tool("none")
    cad.new_design()
    cad.set_camera(target=list(target), dist=dist, yaw=yaw, pitch=pitch, ortho=False)


VIEW = (0, 94, 1034, 706)  # the 3D view at this window size


# --- the key tag used by the tutorial ---------------------------------------------

def tag_plate(cad):
    b = cad.add_shape("box")
    cad.set_param(b, "length", 60)
    cad.set_param(b, "width", 25)
    cad.set_param(b, "height", 3)
    cad.set_param(b, "radius", 2)
    cad.set_position(x=-30, y=-12.5, z=0, ids=[b])
    return b


def tag_hole(cad):
    h = cad.add_shape("cylinder", hole=True)
    cad.set_size(x=5, y=5, z=20, ids=[h])
    cad.set_position(x=-26, y=-2.5, z=-5, ids=[h])
    return h


def tag_text(cad):
    t = cad.add_shape("text")
    cad.set_text(t, "KEYS")
    cad.set_size(x=36, y=11, z=1.5, ids=[t])
    cad.set_position(x=-14, y=-5.5, z=3, ids=[t])
    cad.set_color("#f2c832", ids=[t])
    return t


def key_tag(cad, grouped=True):
    b, h, t = tag_plate(cad), tag_hole(cad), tag_text(cad)
    if not grouped:
        return b, h, t
    g = cad.group("union", ids=[b, h, t])
    cad.set_multicolor(True, ids=[g])
    cad.set_name("Key tag", ids=[g])
    return g


# --- 1: getting started ----------------------------------------------------------

def tour(cad):
    fresh(cad, target=(0, 0, 4), dist=120)
    g = key_tag(cad)
    cad.select([g])
    marks = [
        (1, (300, 26), (262, 26)),
        (2, (190, 104), (140, 74)),
        (3, (700, 104), (740, 76)),
        (4, (1150, 112), (1150, 92)),
        (5, (76, 250), (40, 255)),
        (6, (330, 640)),
        (7, (760, 250), (800, 250)),
        (8, (1006, 330), (1050, 330)),
        (9, (900, 720), (950, 765)),
    ]
    shot(cad, "tour-window", marks=marks)


def tutorial(cad):
    crop = (150, 150, 760, 500)
    # 1: dragging a box in
    fresh(cad, target=(0, 0, 4), dist=120)
    drag_tile(cad, "box", 0, 0, drop=False)
    shot(cad, "tutorial-1-drag", keep_pointer=True)
    cad.mouse_up(*cad.project(0, 0, 0))
    # 2: sized in Properties
    fresh(cad, target=(0, 0, 4), dist=120)
    b = tag_plate(cad)
    cad.select([b])
    shot(cad, "tutorial-2-size", marks=[(1, (770, 386), (800, 386)), (2, (770, 290), (800, 290))])
    # 3: the hole, its size typed
    h = cad.add_shape("cylinder", x=-26, y=-30, hole=True)
    cad.set_size(x=5, y=5, z=10, ids=[h])
    cad.select([h])
    park(cad)
    cad.click(*cad.handle("corner", 1))
    cad.click_control("dim-x")
    shot(cad, "tutorial-3-hole", crop=(120, 280, 700, 500))
    cad.key("esc")
    # 4: aligned
    cad.select([b, h])
    cad.set_tool("align")
    park(cad)
    d = next(d for d in cad.ui_state()["align_dots"] if d["axis"] == "y" and d["where"] == "center")
    cad.mouse_move(d["x"], d["y"])
    save(grab(cad), "tutorial-4-align", crop=(120, 230, 700, 480))
    cad.set_tool("none")
    cad.delete(ids=[h])
    h = tag_hole(cad)
    # 5: the text
    t = tag_text(cad)
    cad.select([t])
    shot(cad, "tutorial-5-text")
    # 6: grouped
    g = cad.group("union", ids=[b, h, t])
    cad.set_multicolor(True, ids=[g])
    cad.select([g])
    shot(cad, "tutorial-6-group", crop=crop)
    # 7: checked
    cad.clear_selection()
    cad.click_control("filament")
    shot(cad, "tutorial-7-check", crop=(380, 94, 660, 420))
    cad.click_control("filament")
    # 8: export
    cad.click_control("export")
    shot(cad, "tutorial-8-export", crop=(360, 141, 560, 518))
    cad.key("esc")


# --- 2: looking around -------------------------------------------------------------

def looking(cad):
    fresh(cad, target=(0, 0, 4), dist=120)
    key_tag(cad)
    cad.clear_selection()
    marks = [(1, (130, 142), (84, 142))]
    for i, id in enumerate(["home", "fit", "zoomin", "zoomout", "ortho", "orbitcenter"]):
        cx, cy = center(cad, id)
        marks.append((i + 2, (130, cy), (cx + 18, cy)))
    shot(cad, "view-controls", crop=(0, 94, 330, 380), marks=marks)
    # perspective and flat
    crop = (170, 230, 500, 380)
    a = view(cad, crop)
    cad.set_camera(ortho=True)
    b = view(cad, crop)
    side_by_side([a, b], "view-perspective-flat", labels=["Perspective", "Flat (orthographic)"])
    # the standard views
    views = []
    for yaw, pitch in [(-90, 89.9), (-90, 0), (0, 0)]:
        cad.set_camera(target=[0, 0, 2], yaw=yaw, pitch=pitch, dist=190, ortho=True)
        views.append(view(cad, (517 - 165, 447 - 110, 330, 220)))
    side_by_side(views, "view-standard", labels=["Top", "Front", "Right"])
    # the center of rotation, shown while orbiting
    cad.set_camera(target=[0, 0, 4], yaw=-112.9, pitch=32, dist=120, ortho=False)
    cad.set_tool("orbit_center")
    x, y = cad.project(-26, 0, 3)
    cad.click(x + 30, y)
    cad.mouse_down(700, 600, button="right")
    cad.mouse_move(690, 596)
    cad.mouse_move(680, 592)
    save(grab(cad), "view-center", crop=(150, 230, 620, 400))
    cad.mouse_up(680, 592, button="right")


# --- 3: shapes ---------------------------------------------------------------------

def shapes(cad):
    fresh(cad)
    marks = [(1, (1010, 122), center(cad, "tab-construction-plane")), (2, (1157, 152), center(cad, "tab-ruler")),
             (3, (1262, 152), (1240, 126)), (4, (1010, 175), (1050, 175)), (5, (1262, 222), center(cad, "search")),
             (6, (1010, 238), (1060, 238)), (7, (1010, 313), (1060, 313)), (8, (1010, 480), (1027, 447))]
    shot(cad, "shapes-panel", crop=(984, 94, 296, 500), marks=marks)
    cad.click_control("library")
    shot(cad, "shapes-library-menu", crop=(1034, 150, 246, 240))
    cad.key("esc")
    cad.click_control("search")
    cad.type_text("gear")
    shot(cad, "shapes-search", crop=(1034, 150, 246, 330))
    cad.key("esc")
    # the Properties panel
    fresh(cad, target=(0, 0, 8), dist=110)
    b = cad.add_shape("box")
    cad.set_param(b, "radius", 3)
    cad.select([b])
    marks = [(1, (770, 124), (804, 124)), (2, (860, 96), (850, 118)), (3, (955, 96), (970, 116)), (4, (1010, 96), (1000, 116)),
             (5, (770, 180), (836, 180)), (6, (1010, 180), (972, 180)), (7, (770, 290), (800, 290)), (8, (770, 386), (800, 386))]
    shot(cad, "shapes-properties", crop=(380, 82, 654, 450), marks=marks)
    # colours
    cad.click_control("insp-solid")
    a = view(cad, (760, 220, 268, 280))
    cad.click_control("color-tab-Custom")
    b2 = view(cad, (504, 190, 530, 460))
    side_by_side([a, b2], "shapes-colors", labels=["Presets", "Custom"])
    cad.key("esc")
    # text and its fonts
    fresh(cad, target=(0, 0, 5), dist=130)
    t = cad.add_shape("text")
    cad.set_text(t, "Hello")
    cad.select([t])
    cad.click_control("param-font")
    # (only the fonts that come with ScottiCAD: the list goes on with the
    # computer's own)
    shot(cad, "shapes-text", crop=(150, 94, 884, 456))
    cad.key("esc")
    # a parametric part
    fresh(cad, target=(0, 0, 4), dist=100)
    g = cad.add_shape("spurgear")
    cad.select([g])
    shot(cad, "shapes-gear")
    # a hole and a solid of the same shape
    fresh(cad, target=(0, 0, 8), dist=180)
    cad.add_shape("cylinder", x=-18)
    cad.add_shape("cylinder", x=18, hole=True)
    cad.clear_selection()
    shot(cad, "shapes-solid-hole", crop=(250, 250, 540, 340))


# --- 4: selecting, moving, sizing ---------------------------------------------------

def moving(cad):
    fresh(cad, target=(0, 0, 9), dist=95)
    b = cad.add_shape("box")
    cad.set_size(x=30, y=20, z=15, ids=[b])
    cad.select([b])
    park(cad)
    hs = {(h["kind"], h["index"]): (h["x"], h["y"]) for h in cad.handles()}
    c, e, t, l, r = hs[("corner", 1)], hs[("edge", 0)], hs[("top", 0)], hs[("lift", 0)], hs[("rotate", 2)]
    marks = [(1, (c[0] + 60, c[1] + 40), c), (2, (e[0] - 10, e[1] + 60), e), (3, (t[0] - 90, t[1] - 10), t),
             (4, (l[0] - 70, l[1] - 50), l), (5, (r[0] + 70, r[1] + 30), r)]
    shot(cad, "move-handles", crop=(160, 150, 620, 520), marks=marks)
    # a dimension being typed
    cad.click(*hs[("corner", 1)])
    cad.click_control("dim-x")
    cad.type_text("42.5")
    shot(cad, "move-type-size", crop=(160, 150, 620, 520))
    cad.key("esc")
    # moving
    x, y = cad.project(0, 0, 8)
    cad.mouse_down(x, y)
    for i in range(1, 7):
        cad.mouse_move(x - 34 * i, y - 4 * i)
    save(grab(cad), "move-drag", crop=(60, 94, 720, 560))
    cad.mouse_up(x - 204, y - 24)
    cad.undo()
    # lifting
    cad.select([b])
    park(cad)
    x, y = cad.handle("lift")
    cad.mouse_down(x, y)
    for i in range(1, 5):
        cad.mouse_move(x, y - 14 * i)
    save(grab(cad), "move-lift", crop=(160, 94, 640, 560))
    cad.mouse_up(x, y - 56)
    cad.undo()
    # rotating
    cad.select([b])
    park(cad)
    x, y = cad.handle("rotate", 2)
    cad.mouse_down(x, y)
    for i in range(1, 6):
        cad.mouse_move(x + 16 * i, y - 7 * i)
    save(grab(cad), "move-rotate", crop=(120, 94, 540, 600))
    cad.mouse_up(x + 80, y - 35)
    cad.undo()
    # several shapes: a marquee, then the selection
    fresh(cad, target=(0, 0, 8), dist=190)
    cad.add_shape("box", x=-30)
    cad.add_shape("cylinder", x=0)
    cad.add_shape("cone", x=30)
    cad.clear_selection()
    x0, y0 = cad.project(-48, -18, 0)
    x1, y1 = cad.project(14, 14, 24)
    cad.mouse_down(x0, y0 + 16)
    for i in range(1, 6):
        cad.mouse_move(x0 + (x1 - x0) * i / 5, y0 + 16 + (y1 - y0 - 16) * i / 5)
    a = grab(cad).crop((250, 260, 250 + 520, 260 + 360))
    cad.mouse_up(x1, y1)
    b2 = view(cad, (250, 260, 520, 360))
    side_by_side([a, b2], "move-marquee", labels=["Drag a box around the shapes...", "...and they are selected together"])
    # the snap grid menu
    cad.clear_selection()
    cad.click_control("snap")
    shot(cad, "move-snap", crop=(800, 540, 234, 260))
    cad.key("esc")
    # hiding
    cad.click_control("visibility")
    shot(cad, "move-visibility-menu", crop=(300, 44, 420, 210))
    cad.key("esc")
    n = cad.nodes()
    cad.hide(ids=[n[1]["id"]])
    cad.set_tool("visibility")
    shot(cad, "move-visibility-mode", crop=(150, 94, 760, 520))
    cad.set_tool("none")


# --- 5: holes and groups -----------------------------------------------------------

def groups(cad):
    crop = (300, 240, 440, 340)
    fresh(cad, target=(0, 0, 8), dist=100)
    b = cad.add_shape("box")
    cad.set_size(x=40, y=30, z=12, ids=[b])
    cad.set_position(x=-20, y=-15, z=0, ids=[b])
    h = cad.add_shape("cylinder", hole=True)
    cad.set_size(x=14, y=14, z=24, ids=[h])
    cad.set_position(x=-7, y=-7, z=-4, ids=[h])
    cad.clear_selection()
    a = view(cad, crop)
    cad.group("union", ids=[b, h])
    cad.clear_selection()
    b2 = view(cad, crop)
    side_by_side([a, b2], "groups-hole", labels=["A solid box and a hole cylinder", "Grouped: the hole is cut away"])
    # the three group types
    imgs = []
    for mode in ["bundle", "union", "intersect"]:
        fresh(cad, target=(0, 0, 10), dist=150)
        b = cad.add_shape("box", x=-6)
        s = cad.add_shape("sphere", x=8)
        cad.set_size(x=26, y=26, z=26, ids=[s])
        cad.set_position(x=-2, y=-13, z=0, ids=[s])
        g = cad.group(mode, ids=[b, s])
        cad.clear_selection()
        imgs.append(view(cad, (347, 290, 340, 300)))
    side_by_side(imgs, "groups-types", labels=["Bundle", "Union", "Intersect"])
    cad.select([g])
    marks = [(1, (818, 340), center(cad, "insp-gt-bundle")), (2, (866, 340), center(cad, "insp-gt-union")),
             (3, (914, 340), center(cad, "insp-gt-intersect"))]
    shot(cad, "groups-type-buttons", crop=(776, 96, 258, 270), marks=marks)
    # one colour, or each part's own
    imgs = []
    for multi in [False, True]:
        fresh(cad, target=(0, 0, 8), dist=100)
        b = cad.add_shape("box", x=-8)
        c = cad.add_shape("cylinder", x=8)
        g = cad.group("union", ids=[b, c])
        cad.set_multicolor(multi, ids=[g])
        cad.clear_selection()
        imgs.append(view(cad, (330, 240, 340, 300)))
    side_by_side(imgs, "groups-multicolor", labels=["One color (the default)", "Multicolor on"])


# --- 6: lining up and measuring ----------------------------------------------------

def precision(cad):
    def trio():
        fresh(cad, target=(0, 0, 8), dist=130)
        a = cad.add_shape("box", x=-32, y=6)
        b = cad.add_shape("cylinder", x=0, y=-8)
        c = cad.add_shape("cone", x=30, y=10)
        return a, b, c
    a, b, c = trio()
    cad.select([a, b, c])
    cad.set_tool("align")
    park(cad)
    dots = cad.ui_state()["align_dots"]
    d = next(d for d in dots if d["axis"] == "y" and d["where"] == "center")
    cad.mouse_move(d["x"], d["y"])
    save(grab(cad), "align-dots", crop=(120, 180, 800, 540))
    cad.set_tool("none")
    # mirror
    fresh(cad, target=(0, 0, 8), dist=110)
    w = cad.add_shape("wedge")
    cad.select([w])
    cad.set_tool("mirror")
    park(cad)
    m = next(m for m in cad.ui_state()["mirror_arrows"] if m["axis"] == "x")
    cad.mouse_move(m["x"], m["y"])
    save(grab(cad), "mirror-arrows", crop=(220, 200, 560, 420))
    cad.set_tool("none")
    # ruler
    fresh(cad, target=(5, 0, 8), dist=130)
    a = cad.add_shape("box", x=-25, y=-5)
    b = cad.add_shape("cylinder", x=20, y=15)
    cad.place_ruler(-35, -15)
    cad.select([b])
    park(cad)
    marks = [(1, (center(cad, "ruler-origin")[0] - 50, center(cad, "ruler-origin")[1] - 30), center(cad, "ruler-origin")),
             (2, (center(cad, "ruler-x-btn")[0] - 40, center(cad, "ruler-x-btn")[1] + 36), center(cad, "ruler-x-btn")),
             (3, (center(cad, "ruler-mid-btn")[0] + 16, center(cad, "ruler-mid-btn")[1] + 40), center(cad, "ruler-mid-btn")),
             (4, (center(cad, "ruler-x")[0] + 10, center(cad, "ruler-x")[1] + 44), center(cad, "ruler-x"))]
    shot(cad, "ruler", crop=(100, 150, 800, 560), marks=marks)
    cad.remove_ruler()
    # measure
    cad.clear_selection()
    cad.set_tool("measure")
    cad.click(*cad.project(-15, 5, 20))
    cad.click(*cad.project(10, 15, 20))
    shot(cad, "measure", crop=(100, 94, 800, 560))
    cad.set_tool("none")
    # a construction plane on a tilted part
    fresh(cad, target=(0, 0, 14), dist=140, pitch=24)
    r = cad.add_shape("box")
    cad.set_size(x=50, y=40, z=6, ids=[r])
    cad.set_position(x=-25, y=-20, z=0, ids=[r])
    cad.rotate("x", 30, ids=[r])
    cad.drop(ids=[r])
    cad.select([r])
    cad.toggle_construction_plane()
    k = cad.add_shape("cylinder", x=0, y=0)
    cad.set_size(x=12, y=12, z=16, ids=[k])
    cad.clear_selection()
    shot(cad, "construction-plane", crop=(100, 94, 860, 620))
    cad.toggle_construction_plane()
    # placing on a surface
    fresh(cad, target=(0, 0, 12), dist=110)
    b = cad.add_shape("box")
    cad.set_size(x=40, y=40, z=14, ids=[b])
    cad.set_position(x=-20, y=-20, z=0, ids=[b])
    cad.clear_selection()
    drag_tile(cad, "cone", 4, 2, 14, drop=False)
    save(grab(cad), "place-on-surface", crop=(150, 94, 720, 520))
    cad.mouse_up(*cad.project(4, 2, 14))
    # lay flat
    fresh(cad, target=(0, 0, 12), dist=170)
    p = cad.add_shape("roof")
    cad.set_size(x=30, y=40, z=24, ids=[p])
    cad.clear_selection()
    cad.set_tool("layflat")
    x, y = cad.project(6, -4, 12)
    cad.mouse_move(x, y)
    a_img = grab(cad).crop((290, 94, 290 + 460, 94 + 520))
    cad.click(x, y)
    cad.set_tool("none")
    cad.clear_selection()
    b_img = view(cad, (290, 94, 460, 520))
    side_by_side([a_img, b_img], "lay-flat", labels=["Click a face...", "...and the part turns to rest on it"])
    # notes
    fresh(cad, target=(0, 0, 8), dist=110)
    b = cad.add_shape("box")
    cad.set_size(x=40, y=30, z=12, ids=[b])
    cad.set_position(x=-20, y=-15, z=0, ids=[b])
    cad.add_note(b, 8, 0, 12, "Print with 4 walls")
    cad.clear_selection()
    a_img = view(cad, (290, 190, 460, 380))
    cad.click(*center(cad, [c["id"] for c in cad.controls() if c["id"].startswith("note-")][0]))
    b_img = view(cad, (290, 190, 460, 380))
    cad.key("esc")
    side_by_side([a_img, b_img], "notes", labels=["A note's marker on a part", "Click the marker to read or edit it"])


# --- 7: split and section ------------------------------------------------------------

def below(cad, n, id):
    """A callout under a control, with a short line up to it."""
    x, y = center(cad, id)
    return (n, (x, y + 42), (x, y + 6))


def cutting(cad):
    fresh(cad, target=(0, 0, 12), dist=110)
    s = cad.add_shape("sphere")
    cad.set_size(x=30, y=30, z=30, ids=[s])
    cad.set_position(x=-15, y=-15, z=0, ids=[s])
    cad.select([s])
    cad.set_tool("split")
    park(cad)
    marks = [below(cad, 1, "split-axis-y"), below(cad, 2, "split-keep-below"), below(cad, 3, "split-gap"),
             (4, (center(cad, "split-grip")[0] - 50, center(cad, "split-grip")[1] - 30), center(cad, "split-grip")),
             below(cad, 5, "split-apply")]
    shot(cad, "split-bar", crop=(80, 94, 700, 500), marks=marks)
    cad.set_tool("none")
    cad.split("z", gap=8)
    cad.clear_selection()
    shot(cad, "split-result", crop=(250, 180, 520, 400))
    # section view of an enclosure
    fresh(cad, target=(0, 0, 12), dist=120)
    p = cad.add_shape("projectbox")
    cad.clear_selection()
    cad.set_section(True, axis="y")
    park(cad)
    marks = [below(cad, 1, "section-axis-y"), below(cad, 2, "section-flip"), below(cad, 3, "section-pin"),
             (4, (center(cad, "section-grip")[0] - 50, center(cad, "section-grip")[1] - 30), center(cad, "section-grip")),
             below(cad, 5, "section-close")]
    shot(cad, "section-view", crop=(120, 94, 760, 560), marks=marks)
    cad.set_section(False)


# --- 8: files ----------------------------------------------------------------------

def files(cad):
    fresh(cad, target=(0, 0, 4), dist=120)
    key_tag(cad)
    cad.clear_selection()
    cad.click_control("filemenu")
    shot(cad, "files-menu", crop=(0, 0, 340, 420))
    cad.click_control("dm-start")
    shot(cad, "files-start")
    cad.key("esc")
    cad.click_control("import")
    shot(cad, "files-import", crop=(380, 180, 520, 440))
    cad.key("esc")
    cad.click_control("export")
    shot(cad, "files-export", crop=(360, 141, 560, 518))
    cad.key("esc")


# --- 9: printing -------------------------------------------------------------------

def printing(cad):
    fresh(cad, target=(0, 0, 4), dist=120)
    g = key_tag(cad)
    cad.clear_selection()
    cad.click_control("filament")
    cad.click_control("fil-settings")
    park(cad)
    shot(cad, "print-filament", crop=(420, 94, 614, 640))
    cad.click_control("fil-settings")
    cad.click_control("filament")
    # a design with things to fix
    fresh(cad, target=(40, 0, 10), dist=210)
    a = cad.add_shape("box", x=-20)
    cad.set_name("Bracket", ids=[a])
    b = cad.add_shape("cylinder", x=20, z=6)
    cad.set_name("Knob", ids=[b])
    c = cad.add_shape("box", x=95)
    cad.set_name("Plate", ids=[c])
    post = cad.add_shape("cylinder", x=60, y=30)
    slab = cad.add_shape("box", x=60, y=30, z=20)
    cad.set_size(x=44, y=44, z=3, ids=[slab])
    cad.set_position(x=48, y=18, z=20, ids=[slab])
    t = cad.group("union", ids=[post, slab])
    cad.set_name("Table", ids=[t])
    cad.clear_selection()
    cad.click_control("filament")
    park(cad)
    shot(cad, "print-check", crop=(120, 94, 914, 560))
    cad.click_control("filament")
    cad.click_control("export")
    shot(cad, "print-export-warning", crop=(360, 141, 560, 518))
    cad.key("esc")


# --- 10: settings ------------------------------------------------------------------

def settings(cad):
    fresh(cad, target=(0, 0, 4), dist=120)
    key_tag(cad)
    cad.clear_selection()
    cad.click_control("settings")
    shot(cad, "settings-dialog", crop=(380, 150, 520, 500))
    cad.click_control("set-preset")
    shot(cad, "settings-presets", crop=(380, 150, 520, 700))
    cad.key("esc")
    cad.key("esc")
    # the themes
    tiles = []
    names = ["Blue", "Green", "Orange", "Purple", "Tan", "Dark"]
    for n in names:
        cad.click_control("settings")
        cad.click_control("set-theme")
        cad.click_control("set-theme-" + n)
        cad.click_control("set-close")
        park(cad)
        tiles.append(grab(cad).resize((416, 260), Image.LANCZOS))
    side_by_side(tiles[:3], "settings-themes-1", labels=names[:3])
    side_by_side(tiles[3:], "settings-themes-2", labels=names[3:])
    cad.click_control("settings")
    cad.click_control("set-theme")
    cad.click_control("set-theme-Tan")
    cad.click_control("set-close")
    # inches
    cad.set_units("in")
    cad.select([cad.nodes()[0]["id"]])
    cad.click(*cad.handle("corner", 1))
    shot(cad, "settings-inches", crop=(150, 180, 884, 620))
    cad.set_units("mm")


# --- 11: scripting -----------------------------------------------------------------

SCRIPT = """# eight pegs in a ring, on a round base
base = cad.add_shape("cylinder")
cad.set_size(x=100, y=100, z=4, ids=[base])
cad.set_position(x=-50, y=-50, z=0, ids=[base])
for i in range(8):
    a = 2 * math.pi * i / 8
    peg = cad.add_shape("cylinder", z=4)
    cad.set_size(x=8, y=8, z=16, ids=[peg])
    cad.move(dx=38 * math.cos(a), dy=38 * math.sin(a), ids=[peg])
print("made", len(cad.nodes()), "shapes")
"""


def scripting(cad):
    fresh(cad, target=(0, 0, 6), dist=210)
    cad.click_control("scripting")
    cad.click_control("con-tab-Console")
    cad.click_control("con-input")
    cad.type_text("cad.add_shape('box')")
    cad.key("enter")
    cad.type_text("print(len(cad.nodes()), 'shape')")
    cad.key("enter")
    cad.type_text("cad.set_size(x=40, ids=[1])")
    settle(cad)
    marks = [(1, (44, 494), (44, 532)), (2, (146, 494), (146, 532)), (3, (420, 726)), (4, (420, 785)),
             (5, (893, 494), (893, 532))]
    shot(cad, "script-console", marks=marks)
    cad.key("ctrl+a")
    cad.key("backspace")
    cad.new_design()
    cad.set_camera(target=[0, 0, 6], dist=210)
    cad.click_control("con-clear")
    cad.click_control("con-tab-Editor")
    cad.answer_dialogs(("message", "no"))
    cad.click_control("con-new")
    cad.click_control("con-editor")
    for line in SCRIPT.rstrip("\n").split("\n"):
        cad.key("home")
        cad.key("shift+end")
        cad.type_text(line)
        cad.key("enter")
    cad.key("f5")
    import time
    for _ in range(200):
        if not cad.console()["running"]:
            break
        time.sleep(0.05)
    cad.clear_selection()
    shot(cad, "script-editor")
    cad.answer_dialogs(("message", "no"))
    cad.click_control("con-new")
    cad.click_control("con-close")


# --- 13: the shape libraries ---------------------------------------------------------

def library_kinds(cad, library):
    """The shapes a library's panel lists, in its order, as (kind, hole),
    scrolling to see them all. Basic Shapes' three hole tiles are left
    out: they are the box, cylinder and sphere again."""
    cad.click_control("library")
    cad.click_control("lib-" + library)
    seen = []
    x, y, w, h = rect(cad, "tiles")
    for _ in range(40):
        new = False
        for c in cad.controls():
            if not c["id"].startswith("tile-"):
                continue
            k = (c["id"][5:], False)
            if c["id"].endswith("-hole"):
                if library == "Basic Shapes":
                    continue
                k = (c["id"][5:-5], True)
            if k not in seen:
                seen.append(k)
                new = True
        cad.wheel(x + w / 2, y + h / 2, -2)
        if not new:
            break
    for _ in range(60):
        cad.wheel(x + w / 2, y + h / 2, 3)
    return seen


def library_sheet(cad, library, name, per_row):
    """A picture of every shape in a library, each scaled to fit a cell (so
    a nut and a rack come out the same size) and named."""
    kinds = library_kinds(cad, library)
    info = {k["id"]: k for k in cad.shape_kinds()}
    fresh(cad)
    cad.set_settings(show_grid=False)
    rows = [kinds[i:i + per_row] for i in range(0, len(kinds), per_row)]
    cell, depth, fit, pitch = 64, 112, 44, 38
    wide, deep = cell * per_row, depth * len(rows)
    cad.set_build_plate(wide + 900, deep + 900)  # its name, at the front edge, is out of the picture
    placed = []
    for r, row in enumerate(rows):
        for c, (k, hole) in enumerate(row):
            x = -wide / 2 + cell * (c + 0.5)
            y = deep / 2 - depth * (r + 0.5)
            id = cad.add_shape(k, x=x, y=y, hole=hole)
            s = cad.node(id)["bounds"]["size"]
            f = min(fit / max(s[0], s[1]), 28 / max(1e-6, s[2]))  # not so tall that it hides the row behind
            cad.set_size(x=s[0] * f, y=s[1] * f, z=s[2] * f, ids=[id])
            n = cad.node(id)["bounds"]["size"]
            cad.set_position(x=x - n[0] / 2, y=y - n[1] / 2, z=0, ids=[id])
            placed.append((k, x, y))
    cad.clear_selection()
    # zoomed so the sheet fills the view: a flat view's scale goes as 1/dist
    want = min(900 / wide, 590 / (deep * math.sin(math.radians(pitch)) + fit))
    cad.set_camera(target=[0, 0, 0], yaw=-90, pitch=pitch, dist=400, ortho=True)
    got = (cad.project(100, 0, 0)[0] - cad.project(0, 0, 0)[0]) / 100
    cad.set_camera(target=[0, -depth * 0.12, 0], dist=400 * got / want)
    park(cad)
    img = grab(cad)
    d = ImageDraw.Draw(img)
    # the view's own buttons, at the left, are painted out
    d.rectangle([0, 94, 70, 470], fill=img.getpixel((20, 520)))
    xs, ys = [], []
    for k, x, y in placed:
        px, py = cad.project(x, y - depth / 2 + 12, 0)
        text = info[k]["label"]
        box = d.textbbox((px, py), text, font=font(13), anchor="mm")
        d.text((px, py), text, font=font(13), fill=(50, 40, 30), anchor="mm")
        xs += [box[0], box[2]]
        ys += [box[1], box[3]]
        for dx in (-cell / 2, cell / 2):
            qx, qy = cad.project(x + dx, y + fit / 2, fit)
            xs.append(qx)
            ys.append(qy)
    x0, y0 = max(0, min(xs) - 10), max(94, min(ys) - 10)
    x1, y1 = min(1034, max(xs) + 10), min(H, max(ys) + 12)
    save(img, name, crop=(x0, y0, x1 - x0, y1 - y0))
    cad.set_settings(show_grid=True)


def libraries(cad):
    library_sheet(cad, "Basic Shapes", "library-basic", 6)
    library_sheet(cad, "Parametric", "library-parametric", 6)
    library_sheet(cad, "Hardware & Enclosures", "library-hardware", 6)
    library_sheet(cad, "Mechanical", "library-mechanical", 6)
    library_sheet(cad, "Computers & Networking", "library-computers", 6)
    cad.click_control("library")
    cad.click_control("lib-Basic Shapes")
    # the bent tube and its table
    fresh(cad, target=(20, 10, 10), dist=150)
    t = cad.add_shape("benttube")
    cad.set_bends(t, [(40, 90), (30, -45, 0), (25, 90, 90), (20,)])
    cad.select([t])
    shot(cad, "library-bent-tube")


# --- 14: the keyboard ---------------------------------------------------------------

def keys(cad):
    fresh(cad)
    cad.key("f1")
    shot(cad, "keys-help", crop=(206, 166, 868, 470))
    cad.key("esc")


CHAPTERS = {"keys": keys, "tour": tour, "tutorial": tutorial, "looking": looking, "shapes": shapes, "moving": moving, "groups": groups,
            "precision": precision, "cutting": cutting, "files": files, "printing": printing, "settings": settings,
            "scripting": scripting, "libraries": libraries}


def main():
    names = sys.argv[1:] or list(CHAPTERS)
    with launch(exe=os.environ.get("SCOTTICAD_EXE"), headless=True, width=W, height=H) as cad:
        for n in names:
            CHAPTERS[n](cad)


if __name__ == "__main__":
    main()
