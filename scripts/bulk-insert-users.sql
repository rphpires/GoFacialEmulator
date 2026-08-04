/* ============================================================================
   W-Access (Invenzi) — PROVISIONAMENTO EM LOTE de Usuários (Cardholders)
   VERSÃO IDEMPOTENTE — pode rodar de novo sem colidir / duplicar.
   ----------------------------------------------------------------------------
   Para cada usuário (loop), faz até 5 passos — cada um só executa se faltar:
     1. CH    : reusa CHMain por IdNumber; se não existe -> wsp_AddCH
     2. Card  : reusa CHCards por CardNumber; se não existe -> wsp_AddCard
     3. CardID: recupera de CHCards por ClearCode/CardNumber (SP não devolve)
     4. Assign: se o cartão ainda não tem CHID -> wsp_AssignCard (só liga cartão↔CH)
     5. Nível : INSERT em CHAccessLevels (CHID+AccessLevelID) se faltar
     6. Flag  : UPDATE CHAux.AuxChk03 = 1 (marca p/ a próxima etapa: envio)

   POR QUE IDEMPOTENTE:
     Um run parcial anterior pode deixar CARTÃO ÓRFÃO (criado por wsp_AddCard mas
     sem CHID porque wsp_AssignCard falhou). Re-rodar do zero colide em
     "O Cartão já está em uso". Esta versão REUSA o cartão órfão e só completa o
     que falta (associação + flag), em vez de recriar.

   GOTCHA wsp_AddCard:
     Expõe só @intError/@strError como OUTPUT — NÃO devolve CardID. Recuperado
     por SELECT em dbo.CHCards (ClearCode é único).

   Campo de flag:
     Coluna correta = CHAux.AuxChk03 (bit). wsp_AddCH cria a linha CHAux junto.

   Regras / defaults:
     - LanguageID = 2 ; CHType = 6 (config) ; PartitionID usuários = 1
     - Card/Id sequenciais: Base + (índice - 1)

   AVISO: schema/assinaturas do W-Access variam por versão. Valide nomes de
   coluna, dbo.CHCards e as assinaturas das SPs no banco do cliente.
   ============================================================================ */

USE [W_ACCESS];
GO

SET NOCOUNT ON;
SET XACT_ABORT ON;

/* ==================================================================== */
/* 1) PARÂMETROS EDITÁVEIS — mexa apenas aqui                           */
/* ==================================================================== */
DECLARE
    @Quantity          int          = 500,                 -- quantos usuários provisionar
    @CHType            smallint     = 6,                  -- tipo do cardholder (filtro da próxima etapa)
    @FirstNamePrefix   varchar(80)  = N'Emulador User ',  -- nome = prefixo + número sequencial
    @BaseIdNumber      bigint       = 100001,             -- IdNumber do 1º usuário (demais = +1)
    @CHPartitionID     int          = 1,                  -- partição do cardholder (usuários ≠ 0/Sistema)
    @LocalityID        smallint     = 3,                  -- localidade do cardholder
    @TemplateStr       varchar(max) = NULL,               -- template biométrico/facial (NULL = sem biometria)

    /* --- cartão --- */
    @BaseCardNumber    bigint       = 100001,             -- CardNumber do 1º cartão (demais = +1)
    @FacilityCode      bigint       = 0,                  -- código de instalação do cartão
    @CardType          smallint     = 0,                  -- tipo do cartão
    @IsAutomaticCard   bit          = 0,
    @CardPartitionID   smallint     = 0,                  -- partição do cartão (DEVE ser 0; ≠0 quebra a associação)
    @LicenseCards      int          = 10000,                  -- contagem de licença
    @ForceFillIPRdrUserID bit       = 0,

    /* --- associação cartão↔usuário (nível de acesso) --- */
    @AccessLevelID     smallint     = 7,                  -- nível de acesso (CfgACAccessLevels). Mesmo das leitoras emuladas.
    @OperatorProfileID smallint     = 1,                  -- perfil do OPERADOR da operação (param @ProfileID da SP). Sempre 1.
    @StartValidity     datetime     = NULL,               -- NULL => GETDATE() no loop
    @EndValidity       datetime     = '2030-12-31 23:59:59';

/* Constante fixa */
DECLARE @LanguageID smallint = 2;   -- convenção do projeto

IF @StartValidity IS NULL SET @StartValidity = GETDATE();

/* ==================================================================== */
/* 1.5) VALIDA @AccessLevelID antes do loop (causa raiz do fail)        */
/* ==================================================================== */
IF @AccessLevelID IS NULL
       OR NOT EXISTS (SELECT 1 FROM dbo.CfgACAccessLevels WHERE AccessLevelID = @AccessLevelID)
BEGIN
    PRINT N'>> ERRO: @AccessLevelID inválido/ausente. Níveis disponíveis:';
    SELECT AccessLevelID, AccessLevelName, PartitionID FROM dbo.CfgACAccessLevels ORDER BY AccessLevelID;
    RAISERROR(N'Defina @AccessLevelID com um nível de acesso válido e rode de novo.', 16, 1);
    RETURN;
END

PRINT N'>> Usando @AccessLevelID = ' + CAST(@AccessLevelID AS varchar(10));

/* ==================================================================== */
/* 2) LOOP DE PROVISIONAMENTO (idempotente)                             */
/* ==================================================================== */
DECLARE
    @i          int = 1,
    @ok         int = 0,
    @fail       int = 0,
    @FirstName  varchar(100),
    @IdNumber   varchar(50),
    @CardNumber bigint,
    @ClearCode  varchar(400),
    @CHID       int,
    @CardID     int,
    @CardOwner  int,
    @newCH      bit,
    @newCard    bit,
    @didAssign  tinyint,   -- 0=já tinha, 1=via SP, 2=via UPDATE direto
    @didLevel   bit,
    @RC         int,
    @intError   smallint,
    @strError   nvarchar(255);

WHILE @i <= @Quantity
BEGIN
    BEGIN TRY
        SET @FirstName  = @FirstNamePrefix + RIGHT(N'0000' + CAST(@i AS varchar(10)), 4);
        SET @IdNumber   = CAST(@BaseIdNumber + (@i - 1) AS varchar(50));
        SET @CardNumber = @BaseCardNumber + (@i - 1);
        SET @ClearCode  = CAST(@CardNumber AS varchar(400));

        SET @CHID = NULL; SET @CardID = NULL; SET @CardOwner = NULL;
        SET @newCH = 0; SET @newCard = 0; SET @didAssign = 0; SET @didLevel = 0;

        /* -------- Passo 1: CH (reusa ou cria) -------- */
        SELECT @CHID = CHID FROM dbo.CHMain
        WHERE IdNumber = @IdNumber AND CHType = @CHType;

        IF @CHID IS NULL
        BEGIN
            EXEC @RC = dbo.wsp_AddCH
                 @CHType, @FirstName, @IdNumber,
                 @CHPartitionID, @LocalityID, @TemplateStr,
                 @CHID OUTPUT;

            IF @RC <> 0 OR @CHID IS NULL
                THROW 50010, N'wsp_AddCH falhou (CHID nulo)', 1;
            SET @newCH = 1;
        END

        /* -------- Passo 2: Card (reusa ou cria) -------- */
        SELECT @CardID = CardID FROM dbo.CHCards
        WHERE CardNumber = @CardNumber AND FacilityCode = @FacilityCode;

        IF @CardID IS NULL
        BEGIN
            SET @intError = 0; SET @strError = NULL;
            EXEC @RC = dbo.wsp_AddCard
                 @ClearCode, @CardNumber, @FacilityCode, @CardType,
                 @IsAutomaticCard, @CardPartitionID, @LicenseCards,
                 @ForceFillIPRdrUserID, @LanguageID,
                 @intError OUTPUT, @strError OUTPUT;

            IF @RC <> 0 OR @intError <> 0
                THROW 50011, N'wsp_AddCard falhou', 1;

            /* Passo 3: recupera CardID (SP não devolve) */
            SELECT @CardID = CardID FROM dbo.CHCards
            WHERE ClearCode = @ClearCode AND FacilityCode = @FacilityCode;

            IF @CardID IS NULL
                THROW 50012, N'CardID não encontrado por ClearCode após wsp_AddCard', 1;
            SET @newCard = 1;
        END

        /* Normaliza a partição do cartão (corrige cartões antigos criados com
           PartitionID errado — partição ≠ 0 quebra a associação cartão↔usuário). */
        UPDATE dbo.CHCards SET PartitionID = @CardPartitionID
        WHERE CardID = @CardID AND PartitionID <> @CardPartitionID;

        /* -------- Passo 4: Assign (só se cartão sem dono) -------- */
        SELECT @CardOwner = CHID FROM dbo.CHCards WHERE CardID = @CardID;

        IF @CardOwner IS NULL
        BEGIN
            SET @intError = 0; SET @strError = NULL;
            /* @ProfileID da SP = perfil do OPERADOR que executa a operação (=1),
               NÃO o nível de acesso do usuário. O nível vai no Passo 5. */
            EXEC @RC = dbo.wsp_AssignCard
                 @CHID, @CardID, @StartValidity, @EndValidity,
                 @OperatorProfileID, @LanguageID,
                 @intError OUTPUT, @strError OUTPUT;

            IF @RC <> 0 OR @intError <> 0
                THROW 50013, N'wsp_AssignCard falhou', 1;
            SET @didAssign = 1;

            /* Fallback: nesta versão wsp_AssignCard nem sempre grava
               CHCards.CHID. Se ainda NULL, vincula por UPDATE direto. */
            SELECT @CardOwner = CHID FROM dbo.CHCards WHERE CardID = @CardID;
            IF @CardOwner IS NULL
            BEGIN
                UPDATE dbo.CHCards SET CHID = @CHID WHERE CardID = @CardID;
                SET @didAssign = 2;
            END
        END
        ELSE IF @CardOwner <> @CHID
            THROW 50014, N'Cartão já pertence a OUTRO CHID', 1;
        /* se @CardOwner = @CHID: já associado, nada a fazer */

        /* -------- Passo 5: nível de acesso (CHAccessLevels, por CHID) --------
           wsp_AssignCard NÃO concede nível — só liga cartão↔CH. O nível é uma
           linha própria em CHAccessLevels (chave CHID+AccessLevelID). */
        IF NOT EXISTS (SELECT 1 FROM dbo.CHAccessLevels
                       WHERE CHID = @CHID AND AccessLevelID = @AccessLevelID)
        BEGIN
            INSERT INTO dbo.CHAccessLevels
                ( CHID, AccessLevelID, AccessLevelStartValidity, AccessLevelEndValidity )
            VALUES
                ( @CHID, @AccessLevelID, @StartValidity, @EndValidity );
            SET @didLevel = 1;
        END

        /* -------- Passo 6: flag p/ a próxima etapa -------- */
        UPDATE dbo.CHAux SET AuxChk03 = 1 WHERE CHID = @CHID;
        IF @@ROWCOUNT = 0
            PRINT N'   [aviso] CHAux inexistente p/ CHID=' + CAST(@CHID AS varchar(10));

        SET @ok = @ok + 1;
        PRINT N'[' + CAST(@i AS varchar(10)) + N'/' + CAST(@Quantity AS varchar(10))
            + N'] OK  ' + @FirstName
            + N' | CHID='  + CAST(@CHID AS varchar(10))
            + N' (' + CASE WHEN @newCH=1 THEN N'novo' ELSE N'reuso' END + N')'
            + N' | CardID=' + CAST(@CardID AS varchar(10))
            + N' (' + CASE WHEN @newCard=1 THEN N'novo' ELSE N'reuso' END + N')'
            + N' | assign=' + CASE @didAssign WHEN 1 THEN N'SP' WHEN 2 THEN N'UPDATE' ELSE N'já tinha' END
            + N' | nível=' + CASE WHEN @didLevel=1 THEN N'add' ELSE N'já tinha' END;
    END TRY
    BEGIN CATCH
        SET @fail = @fail + 1;
        PRINT N'[' + CAST(@i AS varchar(10)) + N'/' + CAST(@Quantity AS varchar(10))
            + N'] ERRO ' + ISNULL(@FirstName, N'(sem nome)')
            + N' -> ' + ERROR_MESSAGE()
            + ISNULL(N' | strError=' + @strError, N'');
    END CATCH

    SET @i = @i + 1;
END

PRINT N'=== Concluído. Sucesso=' + CAST(@ok AS varchar(10))
    + N' Falhas=' + CAST(@fail AS varchar(10)) + N' ===';

/* ==================================================================== */
/* 3) Conferência — usuários do lote + cartão associado                 */
/* ==================================================================== */
--SELECT m.CHID, m.FirstName, m.IdNumber, m.CHType,
--       a.AuxChk03,
--       c.CardID, c.CardNumber, c.ClearCode, c.CHID AS CardCHID
--FROM dbo.CHMain m
--LEFT JOIN dbo.CHAux a   ON a.CHID = m.CHID
--LEFT JOIN dbo.CHCards c ON c.CHID = m.CHID
--WHERE m.CHType = @CHType
--  AND TRY_CAST(m.IdNumber AS bigint) BETWEEN @BaseIdNumber
--                                         AND @BaseIdNumber + @Quantity - 1
--ORDER BY m.IdNumber;
--GO
