import { ChevronLeft } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { createFirmware } from "@gizclaw/adminservice";
import { expectData, toMessage } from "../../components/api";
import { Button } from "@/components/ui/button";
import { ErrorBanner } from "../../components/banners";
import { PageHeader, PageSummaryCard } from "../../components/page-layout";
import { FirmwareEditor, emptyFirmwareForm, formToUpsert } from "./FirmwareForm";

export function FirmwareCreatePage(): JSX.Element {
  const navigate = useNavigate();
  const [form, setForm] = useState(emptyFirmwareForm());
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const create = async (nextForm = form): Promise<void> => {
    setSaving(true);
    setError("");
    try {
      const body = formToUpsert(nextForm);
      const next = await expectData(createFirmware({ body }));
      navigate(`/firmwares/${encodeURIComponent(next.name)}`);
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        actions={
          <Button asChild className="min-w-fit shrink-0 whitespace-nowrap" size="sm" variant="outline">
            <Link to="/firmwares">
              <ChevronLeft className="size-4" />
              Back to list
            </Link>
          </Button>
        }
        items={[{ href: "/overview", label: "Overview" }, { href: "/firmwares", label: "Firmwares" }, { label: "Create" }]}
      />

      <PageSummaryCard
        description="Create a firmware release-line document with rollback, stable, beta, develop, and pending slots."
        eyebrow="Devices"
        title="Create Firmware"
      />

      {error !== "" ? <ErrorBanner message={error} /> : null}

      <FirmwareEditor form={form} onChange={setForm} onSave={(nextForm) => void create(nextForm)} saveLabel="Create" saving={saving} />
    </div>
  );
}
