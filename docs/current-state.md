# BillFlow — Current State

> Updated: 2026-05-12 10:50 +07
> Source of truth checked: local code/migrations/tests, frontend production build, Docker Compose deploy on `192.168.2.109`, production health checks, frontend routes, container uptime, migration logs, and PostgreSQL schema for all three instances.

## Latest Handoff For New Chat

ถ้าเปิดแชทใหม่ ให้เริ่มจากสถานะนี้:

- BillFlow ปกติยังอยู่ที่ `http://192.168.2.109:3010` / backend `8090`.
- Thaisunsport แยก instance อยู่ที่ frontend `3020`, backend `8100`, postgres `5448`.
  - ล่าสุด deploy แล้วเมื่อ 2026-05-11 สำหรับ demo Phase 1 ฝั่งซื้อเท่านั้น.
  - Public URL: `https://pets-mini-museums-ships.trycloudflare.com/login`
  - Health checked: backend `8100` = `ok`, frontend `3020` serve HTML ได้, containers up.
  - Frontend build flags: `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
  - AI model ใน `.env`: `OPENROUTER_MODEL=google/gemini-2.5-flash-lite`, fallback `google/gemini-2.5-flash`.
  - สำหรับ demo Thaisunsport ตอนนี้อย่าเปิด Phase 1+ / Shopee Excel / ใบสั่งขาย จนกว่าลูกค้าจะผ่าน demo ฝั่งซื้อ.
- Henna customer trial ถูกสร้างใหม่จาก BillFlow ปกติ ไม่ใช่ Thaisunsport:
  - Public URL: `https://aurora-enjoyed-backup-lines.trycloudflare.com/login`
  - Frontend `3030`, backend `8110`, postgres `5458`
  - Server folder `/home/bosscatdog/billflow-henna`
  - Containers `billflow-henna-frontend`, `billflow-henna-backend`, `billflow-henna-postgres`
- Instance/port registry อยู่ที่ [deploy-instances.md](deploy-instances.md). ใช้ไฟล์นี้เป็น source of truth เมื่อต้องจำ port/tunnel ของแต่ละร้าน.
- Local Codex skill `buddhist-method` ติดตั้งและอัปเดตแล้วที่ `/Users/nontawatwongnuk/.codex/skills/buddhist-method`; skill ใช้งานได้ใน session นี้และ session ใหม่ควรเห็นจาก skill list.
- `/setup` ถูกยกระดับเป็นหน้าเริ่มต้นใช้งาน:
  - แสดงความพร้อมของ SML, เส้นทางเอกสาร, email, สินค้าใน SML, AI
  - แสดงชื่อร้าน, ฐานข้อมูล SML, AI ที่ใช้งาน, เวลาดึงสินค้า/อ่านอีเมล/นำเข้าล่าสุด
  - แสดงจำนวนเอกสารค้างแยกซื้อ/ขาย/saleinvoice และจำนวน import/log
  - มีปุ่ม `ล้างข้อมูลทดสอบ` สำหรับ admin ล้าง bills/import runs/logs โดยไม่แตะ settings/catalog/mappings/AI usage
  - ตัวเลือกรีเซ็ตเลขรันเอกสารและล้างประวัติอีเมลที่เคยอ่านแล้วต้องเลือกเอง เพราะมีความเสี่ยง doc_no ซ้ำหรืออ่านอีเมลเก่าซ้ำ
  - ถ้าการตั้งค่าหลักพร้อมแล้วแต่ยังมีเอกสารค้าง ระบบจะแยกข้อความเป็น “ระบบพร้อมใช้งาน มีงานค้างให้จัดการ” พร้อมปุ่มไปตรวจเอกสาร/เอกสารพร้อมส่ง/log ที่เกี่ยวข้อง
- BillFlow main deploy แล้วและทดสอบล้างข้อมูลทดสอบจริงแล้ว:
  - bills/import runs ถูกล้างเป็น 0
  - audit logs เหลือ 1 รายการจาก action reset เอง
  - doc counters ไม่ถูก reset
  - ประวัติอีเมลที่เคยอ่านแล้วไม่ถูกล้าง
- UI wording pass ล่าสุดเปลี่ยนคำเทคนิคในหน้าหลักให้เป็นภาษาพนักงานทั่วไปแล้ว เช่น `Reset UAT` → `ล้างข้อมูลทดสอบ`, `UAT Snapshot` → `สรุปข้อมูลทดสอบ`, `AI Control Center` → `การใช้งาน AI`.
- Sidebar ล่าสุดแยก `งานฝั่งซื้อ` และ `งานฝั่งขาย` แล้ว:
  - งานฝั่งซื้อ: `ใบสั่งซื้อ`
  - งานฝั่งขาย: `ใบสั่งขาย`, `ขายสินค้าและบริการ`
- Sidebar badge ล่าสุดนับแยกเมนู ไม่ใช้เลข pending รวมทั้งระบบ:
  - `ใบสั่งซื้อ`: source `shopee_shipped`, bill_type `purchase`
  - `ใบสั่งขาย`: bill_type `sale`, document_route `saleorder` รวม Shopee/Lazada/TikTok Marketplace Excel
  - `ขายสินค้าและบริการ`: bill_type `sale`, document_route `saleinvoice` รวม Shopee/Lazada/TikTok Marketplace Excel
  - นับเฉพาะ `pending + needs_review + failed`
- ทั้ง 3 หน้าเอกสารมีปุ่ม `ส่ง SML ทั้งหมด` สำหรับสถานะ `pending` พร้อม bulk preview/validation ก่อนส่งจริง.
  - Dialog ล่าสุดจัดรายการเอกสารเป็นตารางอ่านง่ายขึ้น แสดงลำดับส่ง, order no, สถานะ, และ `doc_no` ที่คาดว่าจะได้ต่อแถว.
  - เมื่อ backend preview คืนเลขเริ่มต้นเดียวกันหลายบิล frontend จะคำนวณเลขคาดการณ์ตามลำดับส่ง เช่น `...001`, `...002`, `...003`; backend ยังเป็นผู้จองเลขจริงตอนกดส่ง.
  - เพิ่ม `สรุปก่อนส่ง` ให้ตรวจลูกค้า/ผู้ขาย, ปลายทาง, ช่วง doc_no, คลัง/พื้นที่เก็บ, VAT, เวลาเอกสาร ก่อนกดส่ง.
  - หลัง bulk send จบ dialog แสดงผลสำเร็จ/ไม่สำเร็จ/ข้าม พร้อมปุ่มคัดลอก error summary และลิงก์ไปบิลแรกที่ส่งไม่สำเร็จ.
- Shopee Excel import ล่าสุดรองรับปลายทาง SML ทั้ง `saleorder` และ `saleinvoice`; เมื่อ channel default เปลี่ยน endpoint เมนูและข้อความจะเปลี่ยนตาม.
- Shopee Excel status filter ล่าสุดนำเข้าแถวสถานะ `ที่ต้องจัดส่ง` แล้ว; filter ออกเฉพาะ `ยกเลิกแล้ว`.
- Lazada Excel import deployed on BillFlow main + Henna:
  - เพิ่ม `/import/lazada`, `/api/settings/lazada-config`, `/api/import/lazada/preview`, `/api/import/lazada/confirm`, และ `/api/import/lazada/runs`
  - parser อ่านไฟล์ Lazada export จาก `lazada.xlsx`, group ตาม `orderNumber`, ใช้ `paidPrice`, รวมรายการซ้ำเป็น qty, ใช้ `createTime` เป็น `doc_date`, และกรองเฉพาะ `confirmed`, `shipped`, `delivered`
  - สร้าง local bills เป็น `source='lazada'`, `bill_type='sale'`, `document_route='saleorder'` หรือ `saleinvoice` ตาม `/settings/channels`
  - sales queue/badge/setup counts ปรับให้รวม Shopee + Lazada แล้ว
  - deploy แล้วเฉพาะ BillFlow main + BillFlow Henna; Thaisunsport ถูก skip เพราะยังเป็น Phase 1 ฝั่งซื้อ
- TikTok Excel/CSV import deployed on BillFlow main + Henna:
  - เพิ่ม `/import/tiktok`, `/api/settings/tiktok-config`, `/api/import/tiktok/preview`, `/api/import/tiktok/confirm`, และ `/api/import/tiktok/runs`
  - parser รองรับ `.csv` และ `.xlsx` ที่มี header TikTok Seller Center; ไฟล์ตัวอย่างจริงที่ใช้ตรวจคือ `tiktok_csv.csv`
  - ไฟล์ `tiktok_excel.xlsx` ที่ให้มาเป็น workbook ว่าง/template มีเฉพาะ header `Order ID`; parser จะรองรับ XLSX จริงเมื่อ export มี columns ครบ
  - group ตาม `Order ID`, ใช้ `SKU ID` เป็น source SKU fallback เพราะตัวอย่างไม่มี `Seller SKU`, ใช้ `SKU Subtotal After Discount / Quantity` เป็นราคาต่อหน่วย และใช้ `Order Amount` แบบไม่บวกซ้ำเมื่อ order มีหลายแถว
  - กรองเฉพาะสถานะ `จัดส่งแล้ว`/`shipped`/`delivered`; ข้าม `ยกเลิกแล้ว` และ `ที่จะจัดส่ง`
  - สร้าง local bills เป็น `source='tiktok'`, `bill_type='sale'`, `document_route='saleorder'` หรือ `saleinvoice` ตาม `/settings/channels`
  - deploy แล้วเฉพาะ BillFlow main + BillFlow Henna; Thaisunsport ถูก skip เพราะยังเป็น Phase 1 ฝั่งซื้อ
  - Verified after deploy: backend health `8090`, `8110`; frontend routes `/import/tiktok` return HTTP 200 on ports `3010`, `3030`; migration `030_tiktok_import.sql` applied on both
- `ขายสินค้าและบริการ` (`saleinvoice`) ใช้ endpoint ใหม่ `POST /SMLJavaRESTService/saleinvoice/v4`; เปิดใช้งานบน BillFlow main และ BillFlow Henna ส่วน Thaisunsport ยังปิดด้วย Phase 1 feature flags.
- Shopee SKU ถูกเก็บแยกเป็น `bill_items.source_sku`; ถ้า SKU ไม่มีใน SML Catalog จะไม่เอาไปใส่เป็น `item_code`.
- REST SML retry ตรวจซ้ำว่า `item_code` มีอยู่ใน Catalog จริงก่อนส่ง.
- Saleinvoice test ล่าสุด: `BF-INV26050001` ส่ง payload มี `doc_ref_date: "2026-03-10"` แล้ว ถ้า SML UI ไม่แสดงให้ dev SML ตรวจ API mapping.
- UX hardening ล่าสุดสำหรับ Phase 1+:
  - Dashboard เพิ่ม `Action Center` จัดลำดับงานถัดไปให้ user: setup ไม่ครบ, email มีปัญหา, SML fail, mapping ค้าง, และเอกสารพร้อมส่ง
  - หน้า Dashboard และหน้าเอกสารแสดง empty state พร้อมปุ่มไปงานถัดไปเมื่อยังไม่มีบิลหลังล้างข้อมูลทดสอบ
  - `/settings/channels` ซ่อน API path เป็น `รายละเอียดขั้นสูง` เพื่อไม่ให้พนักงานทั่วไปสับสน
  - dialog ส่ง SML ทั้งแบบรายใบและแบบส่งทั้งหมดแสดง field ที่ยังขาด เช่น ลูกค้า/ผู้ขาย, คลัง, พื้นที่เก็บ, ภาษี, เวลาเอกสาร ก่อนกดส่ง
  - bulk send แสดงข้อความเตือนเมื่อมีเอกสารพร้อมส่งเกิน 100 รายการ และแสดง error จาก backend/SML ในแถวที่ส่งไม่สำเร็จ
  - `/settings/email` แสดง `ผู้ส่ง Shopee ที่ยอมรับ` ในตาราง และ backend จะบันทึกคำเตือนภาษาไทยเมื่ออีเมลถูกข้ามเพราะผู้ส่งไม่ตรง
  - `/logs` แสดงคำแนะนำว่า error นั้นผู้ใช้แก้เองได้หรือควรส่งให้ทีมดูแลระบบ/SML API
  - `/logs` error playbook แยกสาเหตุละเอียดขึ้น เช่น SML timeout/network, doc format, ลูกค้า/ผู้ขาย, VAT, คลัง/พื้นที่เก็บ, สินค้า/หน่วย, Gmail App Password, และ AI quota
  - `/logs` แสดง `demo_test_data_reset` เป็น `ล้างข้อมูลทดสอบ` พร้อม badge `Setup`, summary ภาษาไทย, และคำอธิบายว่าเป็นการล้างข้อมูลทดสอบจากหน้า `/setup` ไม่ใช่ error
  - `/api/bills` รองรับทั้ง `per_page` และ `page_size`, และคืน `data: []` แทน `null` เมื่อไม่มีข้อมูล
- Email accepted-sender update:
  - ช่องเดิม `shopee_domains` ใน DB ยังใช้ชื่อเดิมเพื่อเลี่ยง migration เสี่ยง แต่ UI แสดงเป็น `ผู้ส่งที่ยอมรับ`
  - ใส่ได้ทั้งโดเมนและอีเมลเต็ม เช่น `shopee.co.th`, `mail.shopee.co.th`, `billing@example.com`
  - เว้นว่าง = รับทุกผู้ส่งที่ผ่านคำกรองหัวข้อ
  - backend ลด warning ซ้ำจากการ poll อีเมล เพื่อไม่ให้ `/settings/email` แสดง error ยาวเกินจำเป็น
- Gmail IMAP onboarding update:
  - `/settings/email` แสดง checklist 3 ขั้นตอน: เปิด 2-Step Verification, สร้าง Google App Password, เปิด Gmail IMAP
  - มีลิงก์ตรงไป Google Security, App Passwords, และ Gmail POP/IMAP settings
  - ช่องรหัสใน dialog แสดงเป็น `App Password จาก Google` สำหรับ Gmail และอธิบายว่าไม่ใช่รหัสผ่าน Gmail ปกติ
  - Frontend และ backend normalize Gmail App Password โดยลบช่องว่าง/ขีดกลางก่อน test/save/list folders เช่น `qzqq vwqb zydo dtsi` → `qzqqvwqbzydodtsi`
  - ถ้า Gmail App Password หลัง normalize ไม่ครบ 16 ตัว ระบบเตือนก่อนบันทึก/ทดสอบ และ backend reject เพื่อกัน user error
  - `AUTHENTICATIONFAILED` ถูกแปลเป็นคำแนะนำว่าต้องตรวจ App Password, 2-Step Verification, และ IMAP
- Bills email-date display update:
  - อีเมลใหม่ที่ poll หลัง deploy จะเก็บวันที่จาก IMAP envelope header เป็น `raw_data.email_date`
  - ตาราง `/bills` แสดงวันที่อีเมลพร้อม prefix `อีเมล` เมื่อมี `email_date`
  - บิลเก่าที่ไม่มี `email_date` ยัง fallback ไปใช้ `created_at` หรือวันที่เข้าระบบ เพื่อไม่ให้รายการว่างหรือเสีย layout
- IMAP read/unread polling update:
  - IMAP poll ดึงทั้งอีเมลที่ยังไม่อ่านและอ่านแล้วภายใน `lookback_days` ไม่กรองเฉพาะ unread อีกต่อไป
  - ยังใช้ `processed_email_keys` และ bill-level dedup เพื่อกันสร้างบิลซ้ำจากอีเมลเดิมหรือ order เดิม
  - หลัง deploy main poll พบเมล `Fwd: คำสั่งซื้อ #260404V08VQU10 ถูกจัดส่งแล้ว` แล้ว แต่ข้ามเพราะมี dedup tombstone ตั้งแต่ 2026-05-08; หากต้องการทดลองดึงซ้ำหลังล้างข้อมูลทดสอบ ต้องเลือกล้างประวัติอีเมลที่เคยอ่านแล้ว หรือเคลียร์ dedup เฉพาะรายการอย่างตั้งใจ
- IMAP poll-status clarity update:
  - `imap_accounts` มี runtime fields เพิ่ม: `last_poll_found`, `last_poll_processed`, `last_poll_skipped`
  - `/settings/email` เปลี่ยนคอลัมน์จาก `สร้างบิล` เป็น `ผลรอบล่าสุด` และแสดง `พบ / ประมวลผล / ข้าม`
  - `last_poll_messages` ยังเก็บไว้เพื่อ compatibility แต่ความหมายจริงคือจำนวนที่ processor รับไปทำงาน ไม่ใช่จำนวนบิลที่สร้างเสมอ
  - เพิ่ม `last_poll_details` ผ่าน migration `031_imap_poll_details.sql`; UI กดดูรายละเอียดแต่ละเมลได้ว่า subject/from/date คืออะไร, ประมวลผลหรือข้าม, และข้ามเพราะอะไร เช่น เคยประมวลผลแล้ว, ผู้ส่งไม่อยู่ในรายชื่อที่ยอมรับ, หัวข้อไม่ตรงคำกรอง, ไม่พบไฟล์แนบ, หรืออ่านแล้วไม่พบรายการสินค้า
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100` และ frontend `/settings/email` HTTP 200 ทั้ง `3010`, `3030`, `3020`
  - แก้ migration เก่า `002_sml_catalog.sql` และ `004_shopee_shipped.sql` ให้รวม `tiktok` ใน `bills_source_check` ด้วย เพื่อให้ idempotent เมื่อ re-run หลังมีข้อมูล TikTok แล้ว
  - บน BillFlow main เคลียร์ `processed_email_keys` เฉพาะ message/order `260404V08VQU10` แล้ว poll ใหม่สร้างบิล `67c0be5b-9247-4945-9dc0-85ad498243cf` สถานะ `pending`, source `shopee_shipped`, order `#260404V08VQU10`
- Bill Detail item mapping UI update:
  - ในหน้า detail ของบิล เมื่อกดแก้ไขรายการสินค้าแล้วเลือกสินค้าใหม่จาก SML ระบบแสดงรหัส/ชื่อ/คะแนนของสินค้าที่เลือกทันทีในแถวแก้ไข
  - หลังบันทึก ตารางรายการสินค้าจะอัปเดตชื่อสินค้า SML จากตัวเลือกใหม่ทันที ไม่ต้อง refresh หน้า
  - ใช้ร่วมกันกับ purchase, saleorder, และ saleinvoice เพราะเป็น component รายการสินค้าเดียวกัน
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100` และ frontend `/bills` HTTP 200 ทั้ง `3010`, `3030`, `3020`
- Verified Mapping Loop update:
  - เมื่อ user กดบันทึกรายการสินค้า ระบบจะถือเป็นการยืนยัน mapping แม้ `item_code` เป็น code เดิมที่ AI เติมไว้แล้ว แต่แถวนั้นยัง `mapped=false` หรือยังไม่มี `mapping_id`
  - หลังเรียน mapping แล้ว backend จะ apply mapping ไปยังบิลค้าง source/bill_type เดียวกันที่มี `raw_name` ตรงกัน และเลื่อนบิลจาก `needs_review` เป็น `pending` เมื่อทุก item mapped แล้ว
  - แก้ปัญหา Shopee/Lazada/TikTok ที่ชื่อ marketplace ไม่ตรงกับ SML และไฟล์ไม่มี SKU: user ควรจับคู่ครั้งเดียวต่อชื่อสินค้า/ตัวเลือก ไม่ต้องไล่เลือกซ้ำทุกบิล
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100`
  - Henna production data follow-up: applied existing confirmed Shopee mapping to 3 open item rows; promoted 3 saleinvoice bills to `pending`; remaining `needs_review` = 3 raw names/options that still need their first user confirmation
- Bulk Send dialog UX update:
  - Dialog `ส่ง SML ทั้งหมด` ในหน้า `/bills`, `/sales-orders`, และ `/sale-invoices` ปรับ layout ให้สอดคล้องกับ dialog ส่ง SML ใน Bill Detail
  - ใช้กล่องปลายทาง SML, party picker, กล่องตั้งค่าเอกสาร, helper text, advanced Branch/Sale code, หมายเหตุ, และ warning field ที่ขาดในรูปแบบเดียวกัน
  - แสดง `doc_no` preview ต่อบิลในรายการพร้อมส่ง พร้อมเลข order ต้นทาง เพื่อให้ user ตรวจเลขเอกสารก่อนกดส่ง SML ทั้งหมด
  - ส่วนตรวจรายการพร้อมส่งยังคงอยู่เฉพาะ bulk dialog เพราะต้องแสดงจำนวนรายการที่พร้อมส่ง/ต้องข้าม/ผลส่งต่อบิล
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100`; verified frontend routes main/Henna `/sale-invoices` และ Thaisunsport `/bills`; Thaisunsport flags ยังเป็น Phase 1
- Shopee product image update:
  - Email extractor ตัด tracking/open pixel URL ของ Shopee ออก และจัดลำดับให้ product CDN เช่น `cf.shopee.co.th/file/th-*` มาก่อน logo/app/social assets
  - เพิ่ม backend test สำหรับเคสที่อีเมลมี tracking pixel ก่อน product image
  - บิล `67c0be5b-9247-4945-9dc0-85ad498243cf` item `ef15ccb5-c164-476e-84bb-3ef6657e531d` ถูกอัปเดตให้ใช้รูป `https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477`; verified `HTTP 200 image/jpeg`
- SML API line-item update:
  - ทุก payload รายการสินค้าที่ส่งเข้า SML hardcode `is_get_price: 1` ที่ระดับ line item แล้ว
  - ครอบคลุม `sale_reserve` (SML 213 JSON-RPC), `saleorder`, `saleinvoice`, และ `purchaseorder`
  - เพิ่ม backend test `TestSMLLinePayloadsHardcodeIsGetPrice` เพื่อยืนยัน JSON key `is_get_price` มีค่า `1` ในทุก route
- AI model update:
  - production log ยืนยันว่า `google/gemma-4-26b-a4b-it:free` fail แล้ว fallback ไป `google/gemini-2.5-flash-lite`
  - BillFlow main จึงเปลี่ยน model หลักเป็น `google/gemini-2.5-flash-lite` และใช้ `google/gemini-2.5-flash` เป็น fallback สำหรับงานที่ต้องการความเสถียรกว่า
- SML party cache reliability:
  - ตอน backend start ระบบ retry ดึงรายชื่อลูกค้า/ผู้ขายจาก SML หลายรอบแบบ backoff แทนการ fail ครั้งเดียว
  - `/api/sml/parties/last-sync` และ party picker ส่ง/แสดง `status`, `last_attempt`, `last_sync`, `error` เพื่อให้ผู้ใช้รู้ว่าควรกดรีเฟรชหรือตรวจ SML API
  - ตรวจ production ล่าสุดแล้ว `status=ok`, ลูกค้า 1,004 รายการ, ผู้ขาย 500 รายการ

## Next Phase — Shopee API Direct

สถานะก่อนเริ่ม phase ถัดไป:

- Marketplace Excel ทั้ง 3 ช่องทางพร้อมสำหรับ UAT บน BillFlow main + Henna:
  - Shopee Excel
  - Lazada Excel
  - TikTok Excel/CSV
- Thaisunsport ยังเป็น Phase 1 ฝั่งซื้อเท่านั้น และยังไม่ควรเปิด sales/import channel.
- Phase ถัดไปคือเชื่อม Shopee Open Platform API โดยตรง เพื่อดึง order เข้า BillFlow โดยไม่ต้อง export Excel.

แนวทางที่ควรรักษาไว้:

- Shopee API direct ควรสร้าง local bills เข้า pipeline เดิมเหมือน Shopee Excel:
  - `source='shopee'`
  - `bill_type='sale'`
  - `document_route='saleorder'` หรือ `saleinvoice` ตาม `/settings/channels`
  - ใช้ mapping/catalog/review/retry SML flow เดิมทั้งหมด
- Shopee Excel ควรยังคงอยู่เป็น fallback/manual import จนกว่า API direct จะ stable.
- เริ่ม implement/test บน BillFlow main ก่อน แล้วค่อย deploy main + Henna เมื่อผ่าน UAT.
- ไม่ deploy ไป Thaisunsport เว้นแต่ผู้ใช้สั่งเปิด Phase 1+ / งานฝั่งขายให้ร้านนี้.

สิ่งที่ต้องเตรียม/ยืนยันก่อน coding Shopee API:

- Shopee Open Platform app credentials, partner ID/key, redirect URL, และ shop authorization flow.
- Token storage/refresh design: access token, refresh token, expiry, shop_id, shop_name, enabled flag.
- Order statuses ที่ต้อง sync เช่น `READY_TO_SHIP`, `SHIPPED`, `COMPLETED`, และ policy ว่าจะข้าม `CANCELLED` หรือเก็บเป็น skipped run detail.
- Date range/cursor/pagination/rate limit ของ API และ dedup key หลัก (`order_sn` + shop/channel).
- Mapping ระหว่าง Shopee API item fields กับ `bill_items`: item name, variation, model SKU, seller SKU, qty, price, discount, image URL.
- Import run detail ควรแสดงเหมือน Excel/Email: พบกี่ order, สร้างกี่ bill, ข้ามเพราะอะไร.

## Latest Deploy Notes — 2026-05-11

### Main + Henna — Lazada Excel Phase 1+ Sales Flow

- Change type: Phase 1+ / งานฝั่งขาย / Marketplace Excel import.
- Deploy targets: `billflow`, `billflow-henna`.
- Skipped: `billflow-thaisunsport` เพราะยัง Phase 1 ฝั่งซื้อและ sales/Shopee Excel disabled.
- Scope:
  - Lazada Excel preview/confirm endpoints and `/import/lazada` frontend page.
  - Lazada sale bills created as local review items, then sent through existing Bill Detail Retry route.
  - `/settings/channels` and sales queue counters now include Lazada sale routes.
  - Migration `029_lazada_import.sql` adds duplicate guard for `source='lazada'` + `raw_data.order_id`.
- Verification at 2026-05-11 16:42 +07:
  - BillFlow main backend `8090`: `{"database":"ok","env":"production","status":"ok"}`
  - BillFlow Henna backend `8110`: `{"database":"ok","env":"production","status":"ok"}`
  - Frontend `3010` and `3030`: HTTP 200
  - Migration `029_lazada_import.sql` applied on both main and Henna.
  - Thaisunsport backend `8100` remains healthy and its containers were not restarted by this deploy.
- Follow-up at 2026-05-11 16:49 +07:
  - Fixed sidebar/command-palette visibility: Lazada menu no longer depends on `VITE_PHASE >= 2`; it follows `ENABLE_SALES_ORDERS` like Shopee.
  - Rebuilt/restarted frontend only for BillFlow main + Henna.
  - Verified `Lazada Excel` exists in built assets for both frontends and `/import/lazada` returns HTTP 200 on ports `3010` and `3030`.
  - Thaisunsport frontend was not restarted.

### End-of-Day Snapshot

- All three instances are deployed with the latest local backend/frontend/docs state from this session.
- Verified health at 2026-05-11 14:37 +07:
  - BillFlow main backend `8090`: `ok`; frontend `3010`: `HTTP/1.1 200 OK`
  - BillFlow Henna backend `8110`: `ok`; frontend `3030`: `HTTP/1.1 200 OK`
  - BillFlow Thaisunsport backend `8100`: `ok`; frontend `3020`: `HTTP/1.1 200 OK`
- Thaisunsport remains Phase 1 only:
  - `VITE_PHASE=1`
  - `VITE_ENABLE_SALES_ORDERS=false`
  - `VITE_ENABLE_SHOPEE_EXCEL=false`
- BillFlow main verification:
  - Purchase bill `67c0be5b-9247-4945-9dc0-85ad498243cf` exists in `/bills`, status `pending`, `sml_order_id=#260404V08VQU10`.
  - Item `ef15ccb5-c164-476e-84bb-3ef6657e531d` has verified product image URL `https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477`.

### All Instances — SML `is_get_price` Line Item Flag

- Change type: Shared SML API payload requirement.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Added `is_get_price: 1` to every item/detail sent to SML:
  - SML 213 `sale_reserve` JSON-RPC `items`
  - SML 248 REST `saleorder` `items`
  - SML 248 REST `saleinvoice` `details`
  - SML 248 REST `purchaseorder` `items`
- Verification:
  - `go test ./...` passed locally.
  - New test `TestSMLLinePayloadsHardcodeIsGetPrice` asserts the JSON key exists with value `1` for all four routes.
  - Backend deployed and restarted on all three instances.
  - Health checked: `8090`, `8110`, `8100` all return `{"database":"ok","env":"production","status":"ok"}`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.

### All Instances — Shared Phase 1 UX/Logs Clarity

- Change type: Shared Phase 1 UX/Logs clarity.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Deployed local `frontend/` and `backend/` to:
  - `/home/bosscatdog/billflow`
  - `/home/bosscatdog/billflow-henna`
  - `/home/bosscatdog/billflow-thaisunsport`
- Verified health after restart:
  - `8090`, `8110`, `8100` all return `{"database":"ok","env":"production","status":"ok"}`.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- Browser check on BillFlow main `/logs` confirmed `ล้างข้อมูลทดสอบ`, `Setup`, and expanded guidance/facts render correctly.

### All Instances — Gmail IMAP Onboarding

- Change type: Shared Phase 1 email onboarding UX/bug.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Added Gmail checklist and direct setup links in `/settings/email`.
- Added frontend/backend Gmail App Password normalization and validation.
- Added backend unit tests for Gmail App Password separator normalization and short-password rejection.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
  - Browser check on BillFlow main `/settings/email` confirmed checklist, dialog guidance, and `16/16` normalized password hint.

### All Instances — Bills Email Date Display

- Change type: Shared Phase 1 bills/email UX.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- IMAP poll now stores `raw_data.email_date` from the email envelope date for new email-created bills.
- `/bills` displays `email_date` with the prefix `อีเมล` when present, falling back to `created_at` for older bills.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
  - Browser check on BillFlow main `/bills` confirmed the page loads and table/empty state renders.

### All Instances — IMAP Read/Unread Polling

- Change type: Shared Phase 1 email ingestion bug.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- IMAP search now uses the account `lookback_days` window without `NotFlag: Seen`, so both read and unread messages are considered.
- Dedup remains active through `processed_email_keys` and existing bills to prevent duplicate bill creation after reprocessing old mailboxes.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
  - Production log on BillFlow main showed account `fe6a36b8-7092-483a-9265-2e63e248dccb` found 6 messages and received subject `Fwd: คำสั่งซื้อ #260404V08VQU10 ถูกจัดส่งแล้ว`.
  - The same message was skipped by dedup because `processed_email_keys` already had `shopee_shipped` entries for message id `CALTxiT_9pZFi7kCBMgT_2V6FCS8ghNNxkQaVmUbrpaAwULyvbg@mail.gmail.com` from 2026-05-08.

### All Instances — IMAP Poll Status Clarity

- Change type: Shared Phase 1 email settings UX/backend status clarity.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Added migration `028_imap_poll_stats.sql` with `last_poll_found`, `last_poll_processed`, and `last_poll_skipped`.
- `/settings/email` now shows `ผลรอบล่าสุด` as found/processed/skipped instead of labeling `last_poll_messages` as created bills.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport remains Phase 1 with sales/Shopee Excel disabled.
- BillFlow main verification:
  - Removed only the two `processed_email_keys` rows for message id `CALTxiT_9pZFi7kCBMgT_2V6FCS8ghNNxkQaVmUbrpaAwULyvbg@mail.gmail.com` / order `#260404V08VQU10`.
  - Restarted main backend to force immediate poll.
  - Poll created bill `67c0be5b-9247-4945-9dc0-85ad498243cf`, source `shopee_shipped`, status `pending`, `sml_order_id=#260404V08VQU10`.
  - Last poll stats for account `fe6a36b8-7092-483a-9265-2e63e248dccb`: found `6`, processed `4`, skipped `2`, status `ok`.

### All Instances — Shopee Product Image Extraction

- Change type: Shared Phase 1 email ingestion bug.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Fixed `extractShopeeImageURLs` so Shopee tracking/open pixels are excluded and product image CDN URLs are preferred over Shopee logo/app/social assets.
- Added backend test `TestExtractShopeeImageURLsPrefersProductImage`.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Existing BillFlow main item `ef15ccb5-c164-476e-84bb-3ef6657e531d` now stores `https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477`.
  - Server `curl -I` to that URL returned `HTTP/2 200` with `content-type: image/jpeg`.

### Thaisunsport

- Deployed from local BillFlow code to `/home/bosscatdog/billflow-thaisunsport`.
- Scope intentionally limited to Phase 1 purchase flow for same-day customer demo.
- Verified values in server `.env`:
  - `OPENROUTER_MODEL=google/gemini-2.5-flash-lite`
  - `OPENROUTER_FALLBACK_MODEL=google/gemini-2.5-flash`
  - `VITE_PHASE=1`
  - `VITE_ENABLE_SALES_ORDERS=false`
  - `VITE_ENABLE_SHOPEE_EXCEL=false`
- Verified endpoints:
  - `curl http://localhost:8100/health` returns healthy JSON.
  - `curl http://localhost:3020/` returns frontend HTML.
  - Quick Tunnel URL from `/tmp/billflow-thaisunsport-tunnel.log`: `https://pets-mini-museums-ships.trycloudflare.com`
- Backend logs after restart show IMAP polling activity and skipped messages by subject/sender filters; no critical backend startup failure was observed.

### Local Workspace

- Git branch: `main`.
- Last known committed change before this docs update: `21c0815 Harden SML party cache refresh`.
- Untracked customer sample file remains local: `Order.all.20260401_20260430.xlsx`. Do not commit unless explicitly requested.
- Codex skill `buddhist-method` is installed locally and updated with principles 7-16; current session can use it.

## Deployment

| Instance | Server folder | Frontend | Backend | PostgreSQL | Health |
|---|---|---:|---:|---:|---|
| BillFlow main | `/home/bosscatdog/billflow` | `3010` | `8090` | `5438` | ✅ |
| Thaisunsport | `/home/bosscatdog/billflow-thaisunsport` | `3020` | `8100` | `5448` | ✅ |
| Henna | `/home/bosscatdog/billflow-henna` | `3030` | `8110` | `5458` | ✅ |

The server folders are deployed copies, not git checkouts. Deploy/update commands should target the correct folder and must not assume `git status` works there.

Detailed instance registry: [deploy-instances.md](deploy-instances.md).

## Server `.env` Snapshot

Secrets are intentionally omitted.

| Key | Production value observed |
|---|---|
| `OPENROUTER_MODEL` | `google/gemini-2.5-flash-lite` |
| `OPENROUTER_FALLBACK_MODEL` | `google/gemini-2.5-flash` |
| `OPENROUTER_AUDIO_MODEL` | `openai/whisper-1` |
| `SML_BASE_URL` | `http://192.168.2.213:3248` |
| `SHOPEE_SML_URL` | `http://192.168.2.248:8080` |
| `SHOPEE_SML_GUID` | `smlx` |
| `SHOPEE_SML_PROVIDER` | `SMLGOH` |
| `SHOPEE_SML_CONFIG_FILE` | `SMLConfigSMLGOH.xml` |
| `SHOPEE_SML_DATABASE` | `SML1_2026` |
| `SHOPEE_SML_DOC_FORMAT` | `INV` |
| `SHIPPED_SML_DOC_FORMAT` | `PO` |
| `SHOPEE_SML_WH_CODE` / `SHELF_CODE` / `UNIT_CODE` | `WH-01` / `SH-01` / `ถุง` |
| `PUBLIC_BASE_URL` | Cloudflare Quick Tunnel URL, currently configured on server |
| `VITE_API_URL` | `http://192.168.2.109:8090` |
| `VITE_PHASE` | `1` on server frontend build config |

Docker Compose overrides backend `ENV=production`, so `/health` correctly reports production even though `.env` contains `ENV=development`.

## Current Product Behavior

| Area | Current behavior |
|---|---|
| LINE OA | Human inbox at `/messages`, multi-OA CRUD at `/settings/line-oa`, webhook supports `/webhook/line/:oaId` and legacy `/webhook/line`. Old chatbot/cart flow was removed in migration/session 13. |
| Admin chat reply | Uses cached LINE `replyToken` first when available, then falls back to Push API. `delivery_method` records `reply` or `push`. |
| Admin media reply | Uses signed `/public/media/:mediaID` URLs and requires `PUBLIC_BASE_URL` to be reachable by LINE servers. |
| Email | Multi-account IMAP configured in DB via `/settings/email`; no `IMAP_*` env singleton. One goroutine runs per enabled account. |
| Shopee Excel | `/api/import/shopee/preview` parses/dedups and `/api/import/shopee/confirm` creates local bills. SML send happens through bill Retry routing; default sale route is SML 248 `saleorder`, unless channel endpoint explicitly selects `saleinvoice` (`POST /SMLJavaRESTService/saleinvoice/v4`). |
| Lazada Excel | `/api/import/lazada/preview` parses/dedups Lazada export by `orderNumber`, and `/api/import/lazada/confirm` creates local sale bills for the same review/retry flow as Shopee. |
| TikTok Excel/CSV | Deployed main + Henna: `/api/import/tiktok/preview` parses/dedups TikTok `.csv`/`.xlsx` exports by `Order ID`, keeps `Order Amount` order-level to avoid double-counting multi-row orders, and `/api/import/tiktok/confirm` creates local sale bills for the same review/retry flow. |
| Shopee SKU handling | Source SKU from Excel is stored separately as `bill_items.source_sku`. It only becomes SML `item_code` when the same code exists in local SML Catalog; otherwise the row remains needs review. |
| Shopee shipped email | Routes to purchase bill and SML 248 `purchaseorder`. |
| Bill Retry | 4-way dispatch: `sale_reserve`, `saleorder`, `saleinvoice`, `purchaseorder`, selected by source/bill type plus `channel_defaults.endpoint`. Phase 1 purchase send uses the Bill Detail confirmation dialog for supplier, warehouse, shelf, VAT, document time, branch/sale code, and remark. |
| Bulk SML send | `/bills`, `/sales-orders`, and `/sale-invoices` have `ส่ง SML ทั้งหมด` for `pending` documents. It loads a preview, validates each bill, skips invalid rows, shows expected sequential `doc_no` per ready row, and sends ready bills one-by-one using shared dialog values. |
| Mapping dashboard | `/mappings` shows the saved mapping table plus a sidebar hotspot panel for raw product names still appearing in `needs_review` bills, so admins can fix repeated names before they cause more manual review. |
| หน้าเริ่มต้นใช้งาน | `/setup` checks required setup steps, shows shop/system counters, and provides an admin-only test-data reset dialog that preserves settings/catalog/mappings/AI usage by default. |
| Sidebar navigation | Sidebar groups document work by purchase/sale: `งานฝั่งซื้อ` and `งานฝั่งขาย`. Badges are per-document-route queue counts, not global pending count. |
| Bill detail | Shows route preview, blocks send when item validation fails, supports artifacts preview/download, stores optional `bills.remark`, and summarizes the latest SML request/response before raw JSON. |
| Logs | `/logs` shows action-specific summaries. Expanding a row shows key facts first (bill, doc_no, route, trace, error) and keeps raw JSON as a secondary technical view. |
| UX guardrails | Empty queues guide users to import/email setup, channel API details are collapsed, email sender mismatch is surfaced in Thai, and logs classify common SML failures into user-fixable vs support-needed actions. |
| Catalog | SML 248 catalog sync, product create, per-row refresh/delete, embeddings, and in-memory cosine index. |
| SSE | `/api/admin/events` streams inbox/admin events with HMAC token from `/api/admin/events/token`. |
| Background jobs | Daily insight, daily backup, disk monitor, LINE token checker, hourly reply-token cleanup, daily Cloudflare tunnel drift monitor, IMAP coordinator. |

## Database Notes

Local migrations currently run through:

- `001_init.sql` through `031_imap_poll_details.sql`
- Important recent additions: `bill_artifacts`, `chat_conversations.status`, CRM phone/notes/tags, cached reply token, per-OA mark-as-read toggle, `bills.remark`, `app_settings`, document route defaults, processed email keys, AI usage logs, Shopee/Lazada/TikTok import runs, `bills.document_route`, `bill_items.source_sku`, `bill_items.source_image_url`, and `imap_accounts.last_poll_details`.

Production PostgreSQL also contains `system_settings` and `sml_settings`. These tables are not present in the current local migrations and are not referenced by the current codebase, so treat them as legacy leftovers until a future migration either formalizes or removes them.

## Current Document Menus

| Menu | URL | Backend filter | SML route |
|---|---|---|---|
| ใบสั่งซื้อ | `/bills` | source `shopee_shipped`, bill_type `purchase` | `purchaseorder` |
| ใบสั่งขาย | `/sales-orders` | source `shopee`, bill_type `sale`, document_route `saleorder` | `saleorder` |
| ขายสินค้าและบริการ | `/sale-invoices` | source `shopee`, bill_type `sale`, document_route `saleinvoice` | `saleinvoice` v4 |

## Phase 1 Purchase Flow

Phase 1 initially focused on Shopee purchase bills from email. The same review/send pattern is now also used by Shopee Excel sale documents.

1. IMAP account receives Shopee payment/confirmation email.
2. Email coordinator routes the message to `shopee_shipped`.
3. Backend extracts order reference, order date, items, quantities, prices, and source artifacts.
4. Bill is created as purchase bill and appears in `/bills`.
5. Admin reviews item rows, maps or creates SML products when needed.
6. Admin clicks send from Bill Detail.
7. Confirmation dialog requires supplier, warehouse, shelf, VAT type, and VAT rate. Branch code and sale code may be empty and are sent as empty strings.
8. Backend posts to SML REST:

```text
POST http://192.168.2.248:8080/SMLJavaRESTService/v3/api/purchaseorder
```

Required SML headers are read from production config:

| Header | Current value |
|---|---|
| `guid` | `smlx` |
| `provider` | `SMLGOH` |
| `configFileName` | `SMLConfigSMLGOH.xml` |
| `databaseName` | `SML1_2026` |
| `Content-Type` | `application/json; charset=utf-8` |

Purchase payload shape now follows SML v3 transaction attributes:

- Header includes `doc_no`, `doc_date`, `doc_time`, `doc_ref`, `doc_ref_date`, `doc_format_code`, `cust_code`, `supplier_name`, `branch_code`, `sale_code`, `wh_code`, `shelf_code`, `wh_from`, `location_from`, VAT totals, and `items`.
- `user_request` is sent as an empty string for Phase 1 purchaseorder.
- Item lines include `doc_ref`, `item_code`, `item_name`, `unit_code`, `qty`, `price`, `wh_code`, `shelf_code`, `wh_code_2`, `shelf_code_2`, VAT fields, and line totals.
- `branch_code` is included even when empty, because the confirmation dialog is the source of truth.

Known verified result: sending a Shopee purchase bill to SML `purchaseorder` succeeds and creates a document number such as `BF-PO26050002` / `BF-PO26050003`.
