import { useMemo, useState, type ReactNode } from "react";
import {
  ArrowRight,
  BadgeCheck,
  Building2,
  Calculator,
  Check,
  ChevronDown,
  CircleDollarSign,
  FileCheck2,
  LineChart,
  MailCheck,
  MessageCircle,
  Network,
  ReceiptText,
  ShieldCheck,
} from "lucide-react";
import { motion } from "framer-motion";

type ChannelId = "shopee" | "lazada" | "tiktok";
type AddOnId = "email" | "shopeeShipped";

type Channel = {
  id: ChannelId;
  name: string;
  shortName: string;
  logo: string;
  logoClass: string;
  logoColor?: string;
  iconOnly?: boolean;
  status: string;
  tone: string;
  detail: string;
};

type PricePlan = {
  name: string;
  setup: string;
  monthly: string;
  description: string;
  highlighted?: boolean;
};

type Scenario = {
  title: string;
  setup: string;
  oldMonthly: string;
  newMonthly: string;
  increase: string;
};

const setupPrice = 4900;
const marketplaceMonthlyByCount: Record<number, number> = {
  0: 0,
  1: 1290,
  2: 2290,
  3: 2990,
};

const channels: Channel[] = [
  {
    id: "shopee",
    name: "Shopee ฝ่ายขาย",
    shortName: "Shopee",
    logo: "/brand/shopee-logo.svg",
    logoClass: "h-6 w-6",
    logoColor: "#EE4D2D",
    iconOnly: true,
    status: "พร้อมใช้งานจริง",
    tone: "เริ่มใช้งานได้ก่อน",
    detail: "เหมาะกับร้านที่ดึงไฟล์คำสั่งซื้อจาก Shopee แล้วต้องการตรวจบิลก่อนส่งเข้า SML",
  },
  {
    id: "lazada",
    name: "Lazada ฝ่ายขาย",
    shortName: "Lazada",
    logo: "/brand/lazada.svg",
    logoClass: "h-8 w-auto",
    status: "เริ่มจากไฟล์ Excel ก่อน",
    tone: "ยังไม่ใช่การเชื่อมต่ออัตโนมัติเต็มรูปแบบ",
    detail: "ใช้งานผ่านไฟล์ Excel ได้ก่อน และรอใช้ตัวเชื่อมต่อเมื่อพร้อมในแพ็กเดิม",
  },
  {
    id: "tiktok",
    name: "TikTok ฝ่ายขาย",
    shortName: "TikTok",
    logo: "/brand/tiktok.svg",
    logoClass: "h-6 w-6",
    logoColor: "#050505",
    iconOnly: true,
    status: "เริ่มจากไฟล์ Excel/CSV ก่อน",
    tone: "ยังไม่ใช่การเชื่อมต่ออัตโนมัติเต็มรูปแบบ",
    detail: "รองรับไฟล์จาก TikTok ก่อน เหมาะกับร้านที่ต้องการรวมงานตรวจบิลไว้ที่เดียว",
  },
];

const salesPlans: PricePlan[] = [
  {
    name: "1 ช่องทางขาย",
    setup: "4,900",
    monthly: "1,290",
    description: "เริ่มจากช่องทางหลัก เช่น Shopee ฝ่ายขาย",
  },
  {
    name: "2 ช่องทางขาย",
    setup: "4,900",
    monthly: "2,290",
    description: "เหมาะกับร้านที่ขาย 2 แพลตฟอร์ม",
    highlighted: true,
  },
  {
    name: "3 ช่องทางขาย",
    setup: "4,900",
    monthly: "2,990",
    description: "Shopee + Lazada + TikTok ในราคาพิเศษ",
  },
];

const addOns = [
  {
    id: "email" as const,
    name: "อ่านบิลซื้อจากอีเมล",
    description: "อ่านบิลซื้อจากอีเมล, PDF, รูปภาพ หรือไฟล์แนบ",
  },
  {
    id: "shopeeShipped" as const,
    name: "อ่านอีเมลจัดส่งจาก Shopee",
    description: "อ่านอีเมลจัดส่งหรือชำระเงินจาก Shopee แล้วเตรียมเป็นใบสั่งซื้อ",
  },
];

const scenarios: Scenario[] = [
  {
    title: "ขาย Shopee อย่างเดียว",
    setup: "4,900",
    oldMonthly: "-",
    newMonthly: "1,290",
    increase: "เริ่มต้น",
  },
  {
    title: "ใช้ Shopee อยู่แล้ว อยากเพิ่ม Lazada",
    setup: "0",
    oldMonthly: "1,290",
    newMonthly: "2,290",
    increase: "+1,000",
  },
  {
    title: "ใช้ Shopee อยู่แล้ว อยากเพิ่ม Lazada + TikTok",
    setup: "0",
    oldMonthly: "1,290",
    newMonthly: "2,990",
    increase: "+1,700",
  },
  {
    title: "มีบิลซื้อจากอีเมลด้วย",
    setup: "4,900",
    oldMonthly: "1,290",
    newMonthly: "2,580",
    increase: "+1,290",
  },
];

const proofItems = [
  ["ตรวจก่อนส่ง", "ไม่ส่งเข้า SML ทันที"],
  ["จับคู่สินค้า", "จำรหัสสินค้าและหน่วย"],
  ["ดูย้อนหลังได้", "เห็นประวัติและสาเหตุที่ส่งไม่สำเร็จ"],
  ["ทดลอง 30 วัน", "ชำระค่าเริ่มต้น เดือนแรกฟรี"],
];

const priceChips = [
  ["ค่าเริ่มต้น", "4,900 บาท"],
  ["เริ่มต้น", "1,290 / เดือน"],
  ["ทดลองใช้งาน", "30 วัน"],
];

const salesFeatures = [
  ["ระบบตรวจบิลก่อนส่ง SML", "✓", "✓", "✓"],
  ["ส่งเข้า SML ทีละใบและแบบกลุ่ม", "✓", "✓", "✓"],
  ["จับคู่สินค้า SML", "✓", "✓", "✓"],
  ["หน้าภาพรวมและประวัติการทำงาน", "✓", "✓", "✓"],
  ["จำนวนช่องทางขายที่รวมในแพ็ก", "1", "2", "3"],
  ["เพิ่มช่องทางขายภายใน 3 ช่องทาง", "0 ค่าเริ่มต้น", "0 ค่าเริ่มต้น", "0 ค่าเริ่มต้น"],
  ["งานฝั่งซื้อ", "ซื้อเพิ่ม", "ซื้อเพิ่ม", "ซื้อเพิ่ม"],
  ["จำนวนผู้ใช้ระบบ", "2", "3", "5"],
];

const faqs = [
  {
    question: "Lazada และ TikTok พร้อมใช้งานเต็มรูปแบบหรือยัง?",
    answer: "ตอนนี้เริ่มจากไฟล์ Excel/CSV ก่อน ยังไม่ขายว่าเชื่อมต่ออัตโนมัติเต็มรูปแบบ ลูกค้าที่ซื้อแพ็กไว้สามารถรอใช้ต่อได้เมื่อพร้อม",
  },
  {
    question: "ถ้าใช้ Shopee อยู่แล้วเพิ่ม Lazada ต้องเสียค่าเริ่มต้นเพิ่มไหม?",
    answer: "ไม่เสียค่าเริ่มต้นเพิ่มภายใน 3 ช่องทางขาย รายเดือนจะปรับตามจำนวนช่องทางที่ใช้งานจริง",
  },
  {
    question: "BillFlow ส่งเข้า SML ทันทีเลยไหม?",
    answer: "ไม่ส่งทันทีทุกกรณี ระบบให้ตรวจและแก้ข้อมูลก่อนส่งเข้า SML เพื่อลดความเสี่ยงจากข้อมูลผิด",
  },
];

const formatTHB = (amount: number) => new Intl.NumberFormat("th-TH").format(amount);

function BrandLogo({
  name,
  logo,
  logoClass,
  logoColor,
  iconOnly = false,
  showName = true,
}: {
  name: string;
  logo: string;
  logoClass: string;
  logoColor?: string;
  iconOnly?: boolean;
  showName?: boolean;
}) {
  return (
    <div className="brand-logo-plate" aria-label={`${name} logo`}>
      {iconOnly ? (
        <span
          className={`brand-logo-mask ${logoClass}`}
          style={{ ["--logo-url" as string]: `url(${logo})`, ["--logo-color" as string]: logoColor }}
          aria-hidden="true"
        />
      ) : (
        <img src={logo} alt={`${name} logo`} className={logoClass} loading="lazy" />
      )}
      {iconOnly && showName ? <span className="brand-logo-name">{name}</span> : null}
    </div>
  );
}

function SectionIntro({
  eyebrow,
  title,
  children,
}: {
  eyebrow: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="mx-auto mb-8 max-w-3xl text-center">
      <p className="mb-3 text-sm font-semibold text-gold">{eyebrow}</p>
      <h2 className="text-3xl font-semibold text-ivory md:text-[32px]">{title}</h2>
      <p className="mt-4 text-base leading-8 text-muted">{children}</p>
    </div>
  );
}

function MetricBox({
  label,
  value,
  suffix,
  gold = false,
}: {
  label: string;
  value: string;
  suffix: string;
  gold?: boolean;
}) {
  return (
    <div className="metric-box rounded-lg border border-line bg-slate p-4">
      <p className="text-xs font-medium text-muted">{label}</p>
      <p className={`mt-1 text-2xl font-semibold ${gold ? "text-gold" : "text-ivory"}`}>{value}</p>
      <p className="text-xs text-muted">{suffix}</p>
    </div>
  );
}

function PriceCard({ plan }: { plan: PricePlan }) {
  return (
    <motion.article
      initial={{ opacity: 0, y: 10 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-80px" }}
      transition={{ duration: 0.3 }}
      className={`pricing-card relative rounded-lg border bg-charcoal p-6 shadow-card ${
        plan.highlighted ? "border-gold/60 ring-1 ring-gold/20" : "border-line"
      }`}
    >
      {plan.highlighted ? (
        <div className="recommended-badge absolute right-5 top-5 rounded-full bg-gold/10 px-3 py-1 text-xs font-semibold text-gold">
          แนะนำ
        </div>
      ) : null}
      <h3 className="pr-20 text-xl font-semibold text-ivory">{plan.name}</h3>
      <p className="mt-3 min-h-12 text-sm leading-6 text-muted">{plan.description}</p>
      <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
        <MetricBox label="ค่าเริ่มต้น" value={plan.setup} suffix="บาท" />
        <MetricBox label="รายเดือน" value={plan.monthly} suffix="บาท / เดือน" gold />
      </div>
      <div className="mt-5 border-t border-line pt-4 text-sm leading-6 text-muted">
        <div className="flex items-start gap-2">
          <Check className="mt-1 h-4 w-4 shrink-0 text-success" />
          <span>รวมหน้าตรวจบิล, จับคู่สินค้า, ประวัติการทำงาน และส่งเข้า SML</span>
        </div>
        <div className="mt-2 flex items-start gap-2">
          <Check className="mt-1 h-4 w-4 shrink-0 text-success" />
          <span>เพิ่มช่องทางขายภายใน 3 ช่องทาง ไม่เสียค่าเริ่มต้นเพิ่ม</span>
        </div>
      </div>
    </motion.article>
  );
}

function ProductScreens() {
  return (
    <div className="product-stage">
      <div className="product-stage-copy">
        <p>หน้าจอใช้งานจริงของ BillFlow</p>
        <h2>เห็นงานค้าง ตรวจบิล และส่งเข้า SML เฉพาะรายการที่พร้อม</h2>
        <span>ภาพตัวอย่างด้านล่างปิดข้อมูลลูกค้า เลขคำสั่งซื้อ อีเมล และยอดเงินจริงเรียบร้อยแล้ว</span>
      </div>
      <div className="real-screen-grid" aria-label="BillFlow real interface preview">
        <article className="real-screen-card">
          <div className="real-screen-caption">
            <ShieldCheck className="h-5 w-5 text-success" />
            <div>
              <h3>ภาพรวมงานก่อนส่งเข้า SML</h3>
              <p>เห็นงานที่ต้องตรวจ งานพร้อมส่ง และประวัติการทำงานในหน้าเดียว</p>
            </div>
          </div>
          <img
            src="/product/billflow-dashboard-sanitized.png"
            alt="หน้าภาพรวม BillFlow ที่ปิดข้อมูลจริงแล้ว"
            loading="lazy"
          />
        </article>
        <article className="real-screen-card">
          <div className="real-screen-caption">
            <FileCheck2 className="h-5 w-5 text-success" />
            <div>
              <h3>รายการบิลที่พร้อมตรวจและพร้อมส่ง</h3>
              <p>ส่งเข้า SML เฉพาะรายการที่พร้อม ส่วนรายการที่ต้องตรวจจะถูกแยกไว้ชัดเจน</p>
            </div>
          </div>
          <img
            src="/product/billflow-purchase-sanitized.png"
            alt="หน้ารายการใบสั่งซื้อ BillFlow ที่ปิดข้อมูลจริงแล้ว"
            loading="lazy"
          />
        </article>
      </div>
    </div>
  );
}

function PilotNotice() {
  return (
    <div className="pilot-notice">
      <div className="flex items-start gap-3">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-gold text-white">
          <CircleDollarSign className="h-5 w-5" />
        </div>
        <div>
          <p className="text-sm font-semibold text-ivory">ทดลองใช้งานสำหรับลูกค้ารายแรก</p>
          <p className="mt-1 text-sm leading-6 text-muted">
            เริ่มพิสูจน์กับงานจริง 30 วัน: ชำระค่าเริ่มต้น 4,900 บาท และฟรีค่ารายเดือนเดือนแรก หลังจากนั้นคิดรายเดือนตามช่องทางที่ใช้จริง
          </p>
        </div>
      </div>
    </div>
  );
}

function PricingCalculator() {
  const [selectedChannels, setSelectedChannels] = useState<ChannelId[]>(["shopee"]);
  const [selectedAddOns, setSelectedAddOns] = useState<AddOnId[]>([]);

  const selectedCount = selectedChannels.length;
  const marketplaceMonthly = marketplaceMonthlyByCount[selectedCount] ?? 0;
  const marketplaceSetup = selectedCount > 0 ? setupPrice : 0;
  const addOnSetup = selectedAddOns.length * setupPrice;
  const addOnMonthly = selectedAddOns.length * 1290;
  const totalSetup = marketplaceSetup + addOnSetup;
  const totalMonthly = marketplaceMonthly + addOnMonthly;

  const selectedChannelNames = useMemo(
    () => channels.filter((channel) => selectedChannels.includes(channel.id)).map((channel) => channel.shortName),
    [selectedChannels],
  );

  function toggleChannel(id: ChannelId) {
    setSelectedChannels((current) => {
      if (current.includes(id)) {
        return current.length === 1 ? current : current.filter((item) => item !== id);
      }
      return [...current, id];
    });
  }

  function toggleAddOn(id: AddOnId) {
    setSelectedAddOns((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id],
    );
  }

  return (
    <section id="calculator" className="section-shell">
      <div className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
        <div className="calculator-card rounded-lg border border-line bg-charcoal p-5 shadow-card md:p-7">
          <div className="mb-6 flex items-start gap-3">
            <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-steel text-white">
              <Calculator className="h-5 w-5" />
            </div>
            <div>
              <p className="text-sm font-semibold text-gold">คำนวณราคา</p>
              <h2 className="mt-1 text-2xl font-semibold text-ivory md:text-[32px]">
                เลือกช่องทางที่คุณใช้ แล้วดูราคาได้ทันที
              </h2>
              <p className="mt-2 leading-7 text-muted">
                ค่าเริ่มต้นช่องทางขายคงที่ 4,900 บาท ครอบคลุมสูงสุด 3 ช่องทาง
              </p>
            </div>
          </div>

          <div>
            <p className="mb-3 text-sm font-semibold text-ivory">ช่องทางขาย</p>
            <div className="grid gap-3 md:grid-cols-3">
              {channels.map((channel) => {
                const selected = selectedChannels.includes(channel.id);
                return (
                  <button
                    key={channel.id}
                    type="button"
                    onClick={() => toggleChannel(channel.id)}
                    className={`channel-option ${selected ? "channel-option-active" : ""}`}
                    aria-pressed={selected}
                  >
                    <span className="flex items-center justify-between gap-3">
                      <BrandLogo
                        name={channel.shortName}
                        logo={channel.logo}
                        logoClass={channel.logoClass}
                        logoColor={channel.logoColor}
                        iconOnly={channel.iconOnly}
                        showName={channel.id !== "lazada"}
                      />
                      <span className="check-dot">{selected ? <Check className="h-4 w-4" /> : null}</span>
                    </span>
                    <span className="mt-3 block text-xs leading-5 text-muted">{channel.status}</span>
                  </button>
                );
              })}
            </div>
          </div>

          <div className="mt-7">
            <p className="mb-3 text-sm font-semibold text-ivory">งานฝั่งซื้อที่ซื้อเพิ่ม</p>
            <div className="grid gap-3 md:grid-cols-2">
              {addOns.map((item) => {
                const selected = selectedAddOns.includes(item.id);
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => toggleAddOn(item.id)}
                    className={`addon-option ${selected ? "addon-option-active" : ""}`}
                    aria-pressed={selected}
                  >
                    <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate text-steel">
                      <MailCheck className="h-5 w-5" />
                    </span>
                    <span className="text-left">
                      <span className="block font-semibold text-ivory">{item.name}</span>
                      <span className="mt-1 block text-sm leading-6 text-muted">{item.description}</span>
                      <span className="mt-2 block text-xs font-semibold text-gold">+4,900 ค่าเริ่มต้น / +1,290 ต่อเดือน</span>
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
        </div>

        <aside className="quote-panel rounded-lg border border-line bg-charcoal p-5 shadow-card md:sticky md:top-5 md:self-start md:p-7">
          <p className="text-sm font-semibold text-gold">สรุปราคา</p>
          <h3 className="mt-2 text-2xl font-semibold text-ivory">
            {selectedChannelNames.join(" + ") || "เลือกช่องทาง"}
          </h3>
          <div className="mt-6 space-y-4">
            <SummaryRow label="ค่าเริ่มต้นช่องทางขาย" value={`${formatTHB(marketplaceSetup)} บาท`} />
            <SummaryRow label="รายเดือนช่องทางขาย" value={`${formatTHB(marketplaceMonthly)} บาท`} />
            <SummaryRow label="ค่าเริ่มต้นงานฝั่งซื้อ" value={`${formatTHB(addOnSetup)} บาท`} />
            <SummaryRow label="รายเดือนงานฝั่งซื้อ" value={`${formatTHB(addOnMonthly)} บาท`} />
          </div>
          <div className="mt-6 rounded-lg bg-steel p-5 text-white">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-xs text-white/70">ยอดเริ่มต้น</p>
                <p className="mt-1 text-2xl font-semibold">{formatTHB(totalSetup)}</p>
                <p className="text-xs text-white/70">บาท</p>
              </div>
              <div>
                <p className="text-xs text-white/70">รายเดือนรวม</p>
                <motion.p
                  key={totalMonthly}
                  initial={{ opacity: 0, y: 4 }}
                  animate={{ opacity: 1, y: 0 }}
                  className="mt-1 text-2xl font-semibold text-[#F6D28B]"
                >
                  {formatTHB(totalMonthly)}
                </motion.p>
                <p className="text-xs text-white/70">บาท / เดือน</p>
              </div>
            </div>
          </div>
          <p className="mt-4 text-sm leading-6 text-muted">
            เพิ่ม Lazada/TikTok ภายใน 3 ช่องทางขาย ไม่เสียค่าเริ่มต้นเพิ่ม แต่รายเดือนจะปรับตามจำนวนช่องทางที่ใช้จริง
          </p>
        </aside>
      </div>
    </section>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-line pb-3 last:border-0">
      <span className="text-sm text-muted">{label}</span>
      <span className="font-semibold text-ivory">{value}</span>
    </div>
  );
}

function DataTable({
  columns,
  rows,
}: {
  columns: string[];
  rows: string[][];
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-line bg-charcoal">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[680px] border-collapse text-left text-sm">
          <thead>
            <tr className="border-b border-line bg-slate text-ivory">
              {columns.map((column) => (
                <th key={column} className="px-4 py-4 font-semibold">
                  {column}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.join("|")} className="border-b border-line last:border-0">
                {row.map((cell, index) => (
                  <td
                    key={`${cell}-${index}`}
                    className={`px-4 py-4 leading-6 ${index === 0 ? "text-ivory" : "text-muted"}`}
                  >
                    {cell === "✓" ? <Check className="h-4 w-4 text-success" /> : cell}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function App() {
  return (
    <main className="min-h-screen overflow-hidden bg-graphite text-ivory">
      <section className="hero-section border-b border-line bg-white">
        <div className="mx-auto w-full max-w-6xl px-5 py-9 text-center md:px-8 md:py-14 lg:px-10">
          <motion.div
            className="mx-auto max-w-4xl"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.4 }}
          >
            <div>
              <div className="hero-pill mx-auto mb-4 inline-flex items-center gap-2 rounded-full border border-line bg-slate px-4 py-2 text-sm font-medium text-steel md:mb-5">
                <BadgeCheck className="h-4 w-4 text-success" />
                สำหรับร้านที่ใช้ SML และขายผ่าน Shopee / Lazada / TikTok
              </div>
              <h1 className="mx-auto max-w-4xl text-[32px] font-semibold leading-[1.14] text-ivory md:text-5xl md:leading-[1.08]">
                <span className="block">ลดงานคีย์บิลจาก Shopee</span>
                <span className="block">เข้า SML</span>
              </h1>
              <p className="mx-auto mt-4 max-w-3xl text-base font-medium leading-7 text-muted md:text-xl md:leading-8">
                เริ่มจาก Shopee วันนี้ เพิ่ม Lazada/TikTok ได้ภายหลัง โดยตรวจก่อนส่งเข้า SML ทุกครั้ง
              </p>
              <p className="mx-auto mt-3 max-w-3xl text-sm leading-6 text-muted md:text-base md:leading-7">
                ทดลองใช้งาน 30 วัน: ชำระค่าเริ่มต้น 4,900 บาท และฟรีค่ารายเดือนเดือนแรก
              </p>
              <div className="price-chip-row mx-auto mt-6">
                {priceChips.map(([label, value]) => (
                  <div key={label} className="price-chip">
                    <span>{label}</span>
                    <strong>{value}</strong>
                  </div>
                ))}
              </div>
              <div className="mt-6 flex flex-col items-center justify-center gap-3 sm:flex-row md:mt-7">
                <a className="btn-primary" href="#contact">
                  <MessageCircle className="h-4 w-4" />
                  คุยกับทีมทาง LINE
                </a>
                <a className="btn-secondary" href="#calculator">
                  คำนวณราคา
                  <ArrowRight className="h-4 w-4" />
                </a>
              </div>
            </div>
          </motion.div>

          <div className="mx-auto mt-7 grid max-w-[350px] gap-4 text-left md:mt-9 md:max-w-5xl lg:grid-cols-3">
            {salesPlans.map((plan) => (
              <PriceCard key={plan.name} plan={plan} />
            ))}
          </div>

          <div className="proof-strip mx-auto mt-8 max-w-5xl">
            {proofItems.map(([label, body]) => (
              <div key={label} className="proof-item">
                <span>{label}</span>
                <p>{body}</p>
              </div>
            ))}
          </div>

          <div className="mx-auto mt-5 max-w-5xl text-left">
            <PilotNotice />
          </div>

          <div className="mx-auto mt-8 flex max-w-[350px] flex-col justify-center gap-3 sm:max-w-2xl sm:flex-row">
            {channels.map((channel) => (
              <BrandLogo
                key={channel.id}
                name={channel.shortName}
                logo={channel.logo}
                logoClass={channel.logoClass}
                logoColor={channel.logoColor}
                iconOnly={channel.iconOnly}
              />
            ))}
          </div>
        </div>
      </section>

      <section className="section-shell section-ivory pt-0">
        <ProductScreens />
      </section>

      <div className="page-transition">
        <PricingCalculator />
      </div>

      <section id="pricing" className="section-shell section-ivory pt-0">
        <SectionIntro eyebrow="ช่องทางขาย" title="แพ็กเกจขายผ่าน Shopee, Lazada และ TikTok">
          ค่าเริ่มต้นเดียว 4,900 บาท ส่วนรายเดือนคิดตามจำนวนช่องทางที่ใช้งานจริง
        </SectionIntro>
        <div className="grid gap-5 lg:grid-cols-3">
          {salesPlans.map((plan) => (
            <PriceCard key={plan.name} plan={plan} />
          ))}
        </div>
        <p className="mx-auto mt-6 max-w-4xl text-center text-sm leading-7 text-muted">
          ค่าเริ่มต้น 4,900 บาทครอบคลุมช่องทางขายได้ถึง 3 ช่องทาง ส่วนงานฝั่งซื้อและ LINE OA
          คิดเป็นบริการซื้อเพิ่มแยก
        </p>
      </section>

      <section className="section-shell section-graphite bg-slate">
        <SectionIntro eyebrow="ตัวอย่างราคา" title="เลือกตามสิ่งที่คุณใช้อยู่ตอนนี้">
          ตัวอย่างด้านล่างช่วยตอบคำถามว่าถ้าเริ่ม Shopee แล้วเพิ่มช่องทางหรือฝั่งซื้อ ต้องจ่ายเท่าไร
        </SectionIntro>
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {scenarios.map((scenario) => (
            <article key={scenario.title} className="rounded-lg border border-line bg-charcoal p-5 shadow-card">
              <h3 className="min-h-14 text-lg font-semibold leading-7 text-ivory">{scenario.title}</h3>
              <div className="mt-5 grid grid-cols-2 gap-3 text-sm">
                <MetricBox label="ค่าเริ่มต้นเพิ่ม" value={scenario.setup} suffix="บาท" />
                <MetricBox label="เพิ่มจริง" value={scenario.increase} suffix="บาท / เดือน" gold />
                <MetricBox label="รายเดือนเดิม" value={scenario.oldMonthly} suffix="บาท" />
                <MetricBox label="รายเดือนใหม่" value={scenario.newMonthly} suffix="บาท" />
              </div>
            </article>
          ))}
        </div>
      </section>

      <section className="section-shell section-ivory">
        <SectionIntro eyebrow="สถานะช่องทาง" title="สถานะช่องทางที่ขายได้อย่างตรงไปตรงมา">
          Shopee พร้อมใช้งานจริง ส่วน Lazada และ TikTok เริ่มจากไฟล์ Excel/CSV ก่อน
        </SectionIntro>
        <div className="grid gap-5 md:grid-cols-3">
          {channels.map((channel) => (
            <article key={channel.id} className="rounded-lg border border-line bg-charcoal p-6 shadow-card">
              <div className="mb-6 flex items-center justify-between gap-4">
                <BrandLogo
                  name={channel.shortName}
                  logo={channel.logo}
                  logoClass={channel.logoClass}
                  logoColor={channel.logoColor}
                  iconOnly={channel.iconOnly}
                />
                <span className="rounded-full bg-slate px-3 py-1 text-xs font-semibold text-steel">{channel.status}</span>
              </div>
              <h3 className="text-xl font-semibold text-ivory">{channel.name}</h3>
              <p className="mt-3 text-sm font-semibold text-gold">{channel.tone}</p>
              <p className="mt-4 leading-7 text-muted">{channel.detail}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="section-shell section-graphite bg-slate">
        <SectionIntro eyebrow="งานฝั่งซื้อ" title="ฝั่งซื้อคิดแยกเป็นบริการซื้อเพิ่ม">
          เหมาะกับลูกค้าที่ต้องการให้ BillFlow ช่วยอ่านบิลซื้อจากอีเมล หรือสร้างใบสั่งซื้อจากอีเมลจัดส่งของ Shopee
        </SectionIntro>
        <div className="grid gap-5 md:grid-cols-2">
          {addOns.map((plan) => (
            <article key={plan.id} className="rounded-lg border border-line bg-charcoal p-6 shadow-card">
              <MailCheck className="h-6 w-6 text-steel" />
              <h3 className="mt-4 text-xl font-semibold text-ivory">{plan.name}</h3>
              <p className="mt-3 min-h-12 leading-7 text-muted">{plan.description}</p>
              <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
                <MetricBox label="ค่าเริ่มต้น" value="4,900" suffix="บาท" />
                <MetricBox label="รายเดือน" value="1,290" suffix="บาท / เดือน" gold />
              </div>
            </article>
          ))}
        </div>
      </section>

      <section className="section-shell section-ivory">
        <SectionIntro eyebrow="วิธีทำงาน" title="ไม่แทน SML แต่ช่วยลดงานคีย์ซ้ำ">
          BillFlow ช่วยเปลี่ยนไฟล์จากช่องทางขายหรืออีเมล ให้เป็นงานตรวจบิลที่ส่งเข้า SML ได้อย่างเป็นขั้นตอน
        </SectionIntro>
        <div className="grid gap-5 md:grid-cols-2 lg:grid-cols-5">
          {[
            [ReceiptText, "นำไฟล์เข้าระบบ", "นำไฟล์ Shopee/Lazada/TikTok หรือไฟล์แนบอีเมลเข้าระบบ"],
            [FileCheck2, "ตรวจก่อนส่ง", "ตรวจรายการที่ต้องแก้ก่อนส่งเข้า SML"],
            [Network, "จับคู่สินค้า", "จำชื่อสินค้าจากต้นทางกับรหัสสินค้า SML"],
            [Building2, "ส่งเข้า SML", "ตั้งค่าปลายทางเอกสาร คลัง และ VAT ตามงานจริง"],
            [LineChart, "ตรวจย้อนหลัง", "เก็บประวัติ ข้อมูลที่ส่ง และสาเหตุที่ส่งไม่สำเร็จ"],
          ].map(([Icon, title, body]) => (
            <article key={title as string} className="rounded-lg border border-line bg-charcoal p-5 shadow-card">
              <Icon className="h-6 w-6 text-steel" />
              <h3 className="mt-5 font-semibold text-ivory">{title as string}</h3>
              <p className="mt-3 text-sm leading-6 text-muted">{body as string}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="section-shell section-graphite bg-slate">
        <SectionIntro eyebrow="เปรียบเทียบฟีเจอร์" title="รายละเอียดฟีเจอร์">
          เก็บตารางไว้ด้านล่างสำหรับลูกค้าที่ต้องการเทียบรายละเอียดก่อนตัดสินใจ
        </SectionIntro>
        <div className="space-y-4">
          <details className="rounded-lg border border-line bg-charcoal p-5 shadow-card">
            <summary className="flex cursor-pointer items-center justify-between gap-4 text-lg font-semibold text-ivory">
              ดูตารางเปรียบเทียบแพ็กเกจ
              <ChevronDown className="h-5 w-5 text-muted" />
            </summary>
            <div className="mt-5">
              <DataTable columns={["ฟีเจอร์", "1 ช่องทาง", "2 ช่องทาง", "3 ช่องทาง"]} rows={salesFeatures} />
            </div>
          </details>
          <details className="rounded-lg border border-line bg-charcoal p-5 shadow-card">
            <summary className="flex cursor-pointer items-center justify-between gap-4 text-lg font-semibold text-ivory">
              คำถามที่พบบ่อย
              <ChevronDown className="h-5 w-5 text-muted" />
            </summary>
            <div className="mt-5 grid gap-4 md:grid-cols-3">
              {faqs.map((item) => (
                <div key={item.question} className="border-l-2 border-gold pl-4">
                  <h3 className="font-semibold text-ivory">{item.question}</h3>
                  <p className="mt-2 text-sm leading-6 text-muted">{item.answer}</p>
                </div>
              ))}
            </div>
          </details>
        </div>
      </section>

      <section id="contact" className="px-5 pb-20 md:px-8 lg:px-10">
        <div className="mx-auto max-w-6xl rounded-lg border border-line bg-steel p-8 text-white shadow-card md:p-10">
          <div className="grid gap-8 md:grid-cols-[1fr_auto] md:items-center">
            <div>
              <p className="text-sm font-semibold text-[#F6D28B]">เริ่มจากงานที่มีบิลเยอะที่สุดก่อน</p>
              <h2 className="mt-3 text-3xl font-semibold md:text-4xl">
                ให้ทีม BillFlow ช่วยดูว่าแพ็กไหนเหมาะกับร้านคุณ
              </h2>
              <p className="mt-4 max-w-3xl leading-8 text-white/80">
                ส่งตัวอย่างไฟล์ Shopee, Lazada, TikTok หรือบิลซื้อจากอีเมลมาให้ดูได้ ทีมงานจะช่วยแนะนำจุดเริ่มต้น
              </p>
              <p className="mt-3 text-sm text-white/70">LINE: เพิ่มลิงก์จริงภายหลัง</p>
            </div>
            <a className="btn-primary btn-on-dark w-full justify-center md:w-auto" href="#contact">
              <MessageCircle className="h-4 w-4" />
              คุยกับทีมทาง LINE
            </a>
          </div>
        </div>
      </section>
    </main>
  );
}

export default App;
